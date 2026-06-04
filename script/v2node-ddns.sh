#!/bin/bash

set -u
set -o pipefail

CONFIG_FILE="${V2NODE_DDNS_CONFIG:-/etc/v2node/ddns.env}"
STATE_DIR="${V2NODE_DDNS_STATE_DIR:-/var/lib/v2node}"
STATE_FILE="${STATE_DIR}/ddns.state"
LOCK_DIR="${V2NODE_DDNS_LOCK_DIR:-/run/v2node-ddns.lock}"
LOG_FILE="${V2NODE_DDNS_LOG:-/var/log/v2node-ddns.log}"

log() {
    local message="$*"
    mkdir -p "$(dirname "$LOG_FILE")" >/dev/null 2>&1 || true
    printf '%s %s\n' "$(date '+%F %T')" "$message" | tee -a "$LOG_FILE"
}

fail() {
    log "ERROR: $*"
    exit 1
}

usage() {
    cat <<EOF
v2node-ddns usage:
  v2node-ddns run       执行一次 DDNS 更新和墙检测
  v2node-ddns status    查看配置摘要和最近状态
  v2node-ddns help      显示帮助

配置文件: ${CONFIG_FILE}
日志文件: ${LOG_FILE}
状态文件: ${STATE_FILE}
EOF
}

load_config() {
    if [[ ! -f "$CONFIG_FILE" ]]; then
        echo "DDNS 配置不存在: $CONFIG_FILE" >&2
        return 1
    fi

    # shellcheck source=/dev/null
    set -a
    . "$CONFIG_FILE"
    set +a

    : "${CF_RECORD_TYPE:=A}"
    : "${CF_TTL:=1}"
    : "${CF_PROXIED:=false}"
    : "${CHECK_INTERVAL_MINUTES:=5}"
    : "${BLOCK_CHECK_ENABLED:=false}"
    : "${BLOCK_CHECK_TIMEOUT:=10}"
    : "${BLOCK_CHECK_FAIL_THRESHOLD:=3}"
    : "${CHANGE_IP_WAIT_SECONDS:=60}"
    : "${CHANGE_IP_COOLDOWN_SECONDS:=1800}"

    [[ -n "${CF_API_TOKEN:-}" ]] || return 1
    [[ -n "${CF_ZONE_ID:-}" ]] || return 1
    [[ -n "${CF_RECORD_NAME:-}" ]] || return 1

    CF_RECORD_TYPE="$(echo "$CF_RECORD_TYPE" | tr '[:lower:]' '[:upper:]')"
    if [[ "$CF_RECORD_TYPE" != "A" && "$CF_RECORD_TYPE" != "AAAA" ]]; then
        echo "CF_RECORD_TYPE 只支持 A 或 AAAA" >&2
        return 1
    fi

    [[ "$CF_TTL" =~ ^[0-9]+$ ]] || CF_TTL=1
    [[ "$BLOCK_CHECK_TIMEOUT" =~ ^[0-9]+$ ]] || BLOCK_CHECK_TIMEOUT=10
    [[ "$BLOCK_CHECK_FAIL_THRESHOLD" =~ ^[0-9]+$ ]] || BLOCK_CHECK_FAIL_THRESHOLD=3
    [[ "$CHANGE_IP_WAIT_SECONDS" =~ ^[0-9]+$ ]] || CHANGE_IP_WAIT_SECONDS=60
    [[ "$CHANGE_IP_COOLDOWN_SECONDS" =~ ^[0-9]+$ ]] || CHANGE_IP_COOLDOWN_SECONDS=1800
}

require_cmd() {
    local missing=()
    for cmd in "$@"; do
        command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        fail "缺少命令: ${missing[*]}，请先运行 v2node update 或安装 jq/curl"
    fi
}

normalize_bool() {
    case "$(echo "${1:-false}" | tr '[:upper:]' '[:lower:]')" in
        1|true|yes|y|on) echo "true" ;;
        *) echo "false" ;;
    esac
}

is_valid_ip() {
    local ip="$1"
    if [[ "$CF_RECORD_TYPE" == "AAAA" ]]; then
        [[ "$ip" == *:* ]]
    else
        [[ "$ip" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]
    fi
}

curl_ip() {
    local url="$1"
    local family_arg="-4"
    [[ "$CF_RECORD_TYPE" == "AAAA" ]] && family_arg="-6"
    curl "$family_arg" -fsS --max-time 10 "$url" 2>/dev/null | tr -d '[:space:]'
}

get_public_ip() {
    local ip=""
    if [[ "$CF_RECORD_TYPE" == "AAAA" ]]; then
        for url in \
            "https://api64.ipify.org" \
            "https://ifconfig.co/ip" \
            "https://icanhazip.com"; do
            ip="$(curl_ip "$url" || true)"
            if is_valid_ip "$ip"; then
                echo "$ip"
                return 0
            fi
        done
    else
        for url in \
            "https://api.ipify.org" \
            "https://ipv4.icanhazip.com" \
            "https://ifconfig.me/ip"; do
            ip="$(curl_ip "$url" || true)"
            if is_valid_ip "$ip"; then
                echo "$ip"
                return 0
            fi
        done
    fi
    return 1
}

cf_headers() {
    curl -fsS \
        -H "Authorization: Bearer ${CF_API_TOKEN}" \
        -H "Content-Type: application/json" \
        "$@"
}

cf_get_record() {
    cf_headers -G "https://api.cloudflare.com/client/v4/zones/${CF_ZONE_ID}/dns_records" \
        --data-urlencode "type=${CF_RECORD_TYPE}" \
        --data-urlencode "name=${CF_RECORD_NAME}"
}

cf_upsert_record() {
    local ip="$1"
    local proxied payload response success record_id old_ip

    proxied="$(normalize_bool "${CF_PROXIED}")"
    response="$(cf_get_record)" || {
        log "Cloudflare 查询 DNS 记录失败"
        return 1
    }

    success="$(echo "$response" | jq -r '.success // false')"
    [[ "$success" == "true" ]] || {
        log "Cloudflare API 返回失败: $(echo "$response" | jq -c '.errors // []')"
        return 1
    }

    record_id="$(echo "$response" | jq -r '.result[0].id // empty')"
    old_ip="$(echo "$response" | jq -r '.result[0].content // empty')"
    if [[ "$old_ip" == "$ip" && -n "$record_id" ]]; then
        log "DDNS 无需更新: ${CF_RECORD_NAME} ${CF_RECORD_TYPE} ${ip}"
        return 0
    fi

    payload="$(jq -n \
        --arg type "$CF_RECORD_TYPE" \
        --arg name "$CF_RECORD_NAME" \
        --arg content "$ip" \
        --argjson ttl "${CF_TTL}" \
        --argjson proxied "$proxied" \
        '{type:$type,name:$name,content:$content,ttl:$ttl,proxied:$proxied}')"

    if [[ -n "$record_id" ]]; then
        response="$(cf_headers -X PUT \
            "https://api.cloudflare.com/client/v4/zones/${CF_ZONE_ID}/dns_records/${record_id}" \
            --data "$payload")" || {
            log "Cloudflare 更新 DNS 记录失败"
            return 1
        }
    else
        response="$(cf_headers -X POST \
            "https://api.cloudflare.com/client/v4/zones/${CF_ZONE_ID}/dns_records" \
            --data "$payload")" || {
            log "Cloudflare 创建 DNS 记录失败"
            return 1
        }
    fi

    success="$(echo "$response" | jq -r '.success // false')"
    if [[ "$success" == "true" ]]; then
        log "DDNS 已更新: ${CF_RECORD_NAME} ${CF_RECORD_TYPE} ${ip}"
        return 0
    fi

    log "Cloudflare API 返回失败: $(echo "$response" | jq -c '.errors // []')"
    return 1
}

load_state() {
    FAIL_COUNT=0
    LAST_CHANGE_TS=0
    LAST_IP=""
    if [[ -f "$STATE_FILE" ]]; then
        # shellcheck source=/dev/null
        . "$STATE_FILE" || true
    fi
}

save_state() {
    mkdir -p "$STATE_DIR" >/dev/null 2>&1
    umask 077
    cat > "$STATE_FILE" <<EOF
FAIL_COUNT=${FAIL_COUNT:-0}
LAST_CHANGE_TS=${LAST_CHANGE_TS:-0}
LAST_IP='${LAST_IP:-}'
UPDATED_AT='$(date '+%F %T')'
EOF
}

render_check_url() {
    local ip="$1"
    local url="${BLOCK_CHECK_URL:-}"
    url="${url//\{ip\}/$ip}"
    url="${url//\{domain\}/${CF_RECORD_NAME}}"
    echo "$url"
}

check_blocked() {
    local ip="$1"
    local enabled url response keyword

    enabled="$(normalize_bool "${BLOCK_CHECK_ENABLED}")"
    [[ "$enabled" == "true" ]] || return 1
    [[ -n "${BLOCK_CHECK_URL:-}" ]] || return 1

    url="$(render_check_url "$ip")"
    response="$(curl -fsS --max-time "${BLOCK_CHECK_TIMEOUT}" "$url" 2>&1)"
    if [[ $? -ne 0 ]]; then
        log "墙检测接口请求失败，计为异常: ${url} -> ${response}"
        return 0
    fi

    keyword="${BLOCK_CHECK_BLOCKED_KEYWORD:-}"
    if [[ -n "$keyword" ]]; then
        if grep -Fq "$keyword" <<< "$response"; then
            log "墙检测命中关键词: ${keyword}"
            return 0
        fi
        return 1
    fi

    return 1
}

run_change_ip_command() {
    [[ -n "${CHANGE_IP_CURL_CMD:-}" ]] || {
        log "未配置换 IP curl 命令，跳过自动换 IP"
        return 1
    }

    log "开始执行换 IP curl 命令"
    bash -c "$CHANGE_IP_CURL_CMD" >> "$LOG_FILE" 2>&1
    local code=$?
    if [[ $code -ne 0 ]]; then
        log "换 IP curl 命令执行失败，退出码: ${code}"
        return $code
    fi
    log "换 IP curl 命令执行完成，等待 ${CHANGE_IP_WAIT_SECONDS}s 后刷新 DDNS"
    sleep "${CHANGE_IP_WAIT_SECONDS}"
}

with_lock() {
    if ! mkdir "$LOCK_DIR" 2>/dev/null; then
        log "已有 v2node-ddns 实例运行，跳过本次执行"
        exit 0
    fi
    trap 'rmdir "$LOCK_DIR" >/dev/null 2>&1 || true' EXIT
}

run_once() {
    with_lock
    load_config || fail "DDNS 配置不完整，请运行 v2node ddns 重新配置"
    require_cmd curl jq
    load_state

    local ip now threshold cooldown
    ip="$(get_public_ip)" || fail "获取当前公网 IP 失败"
    LAST_IP="$ip"
    cf_upsert_record "$ip" || fail "Cloudflare DDNS 更新失败"

    if check_blocked "$ip"; then
        FAIL_COUNT=$((FAIL_COUNT + 1))
        threshold="${BLOCK_CHECK_FAIL_THRESHOLD:-3}"
        cooldown="${CHANGE_IP_COOLDOWN_SECONDS:-1800}"
        now="$(date +%s)"
        log "墙检测异常次数: ${FAIL_COUNT}/${threshold}"

        if (( FAIL_COUNT >= threshold )); then
            if (( LAST_CHANGE_TS > 0 && now - LAST_CHANGE_TS < cooldown )); then
                log "仍在换 IP 冷却期，剩余 $((cooldown - (now - LAST_CHANGE_TS)))s"
            else
                if run_change_ip_command; then
                    LAST_CHANGE_TS="$(date +%s)"
                    FAIL_COUNT=0
                    ip="$(get_public_ip)" || fail "换 IP 后获取公网 IP 失败"
                    LAST_IP="$ip"
                    cf_upsert_record "$ip" || fail "换 IP 后 Cloudflare DDNS 更新失败"
                fi
            fi
        fi
    else
        if (( FAIL_COUNT > 0 )); then
            log "墙检测恢复正常，清零异常计数"
        fi
        FAIL_COUNT=0
    fi

    save_state
}

mask() {
    local value="${1:-}"
    if [[ ${#value} -le 8 ]]; then
        echo "***"
    else
        echo "${value:0:4}***${value: -4}"
    fi
}

show_status() {
    if ! load_config; then
        echo "DDNS: 未配置或配置不完整"
        return 1
    fi
    load_state
    echo "DDNS 配置: ${CONFIG_FILE}"
    echo "Cloudflare Record: ${CF_RECORD_NAME} ${CF_RECORD_TYPE}"
    echo "Zone ID: $(mask "${CF_ZONE_ID}")"
    echo "API Token: $(mask "${CF_API_TOKEN}")"
    echo "Proxied: $(normalize_bool "${CF_PROXIED}")"
    echo "检查间隔: ${CHECK_INTERVAL_MINUTES} 分钟"
    echo "墙检测: $(normalize_bool "${BLOCK_CHECK_ENABLED}")"
    if [[ -n "${BLOCK_CHECK_URL:-}" ]]; then
        echo "墙检测接口: ${BLOCK_CHECK_URL}"
    fi
    echo "异常计数: ${FAIL_COUNT:-0}/${BLOCK_CHECK_FAIL_THRESHOLD:-3}"
    echo "最近 IP: ${LAST_IP:-}"
    echo "最近换 IP 时间戳: ${LAST_CHANGE_TS:-0}"
    echo "日志: ${LOG_FILE}"
}

case "${1:-run}" in
    run) run_once ;;
    status) show_status ;;
    help|-h|--help) usage ;;
    *) usage; exit 1 ;;
esac
