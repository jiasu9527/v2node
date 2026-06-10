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
    printf '%s %s\n' "$(date '+%F %T')" "$message" | tee -a "$LOG_FILE" >&2
}

fail() {
    log "ERROR: $*"
    exit 1
}

usage() {
    cat <<EOF
v2node-ddns usage:
  v2node-ddns run       执行一次 DDNS 更新和/或墙检测
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
    : "${CHECK_INTERVAL_MINUTES:=1}"
    : "${DDNS_UPDATE_ENABLED:=false}"
    : "${BLOCK_CHECK_ENABLED:=false}"
    : "${BLOCK_CHECK_TIMEOUT:=10}"
    : "${BLOCK_CHECK_FAIL_THRESHOLD:=1}"
    : "${CF_RETRY_ATTEMPTS:=5}"
    : "${CF_RETRY_DELAY:=3}"
    : "${CHANGE_IP_WAIT_SECONDS:=0}"
    : "${CHANGE_IP_COOLDOWN_SECONDS:=0}"

    CF_RECORD_TYPE="$(echo "$CF_RECORD_TYPE" | tr '[:lower:]' '[:upper:]')"
    if [[ "$CF_RECORD_TYPE" != "A" && "$CF_RECORD_TYPE" != "AAAA" ]]; then
        echo "CF_RECORD_TYPE 只支持 A 或 AAAA" >&2
        return 1
    fi

    CF_RECORD_NAME="${CF_RECORD_NAME:-}"
    CF_RECORD_NAME="${CF_RECORD_NAME%.}"
    CF_RECORD_NAME="$(echo "$CF_RECORD_NAME" | tr '[:upper:]' '[:lower:]')"

    [[ "$CF_TTL" =~ ^[0-9]+$ ]] || CF_TTL=1
    if [[ "$(normalize_bool "${DDNS_UPDATE_ENABLED}")" == "true" ]]; then
        [[ -n "${CF_API_TOKEN:-}" ]] || return 1
        [[ -n "${CF_RECORD_NAME:-}" ]] || return 1
    fi
    [[ "$BLOCK_CHECK_TIMEOUT" =~ ^[0-9]+$ ]] || BLOCK_CHECK_TIMEOUT=10
    [[ "$BLOCK_CHECK_FAIL_THRESHOLD" =~ ^[0-9]+$ ]] || BLOCK_CHECK_FAIL_THRESHOLD=1
    [[ "$CF_RETRY_ATTEMPTS" =~ ^[0-9]+$ ]] || CF_RETRY_ATTEMPTS=5
    [[ "$CF_RETRY_DELAY" =~ ^[0-9]+$ ]] || CF_RETRY_DELAY=3
    [[ "$CHANGE_IP_WAIT_SECONDS" =~ ^[0-9]+$ ]] || CHANGE_IP_WAIT_SECONDS=0
    [[ "$CHANGE_IP_COOLDOWN_SECONDS" =~ ^[0-9]+$ ]] || CHANGE_IP_COOLDOWN_SECONDS=0
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
    curl -sS --connect-timeout 10 --max-time 30 \
        -H "Authorization: Bearer ${CF_API_TOKEN}" \
        -H "Content-Type: application/json" \
        "$@"
}

cf_errors() {
    local response="$1"
    echo "$response" | jq -c '.errors // []' 2>/dev/null || echo "${response:0:300}"
}

cf_success() {
    local response="$1"
    [[ "$(echo "$response" | jq -r '.success // false' 2>/dev/null)" == "true" ]]
}

cf_retryable_response() {
    local response="$1"
    if ! echo "$response" | jq -e . >/dev/null 2>&1; then
        return 0
    fi
    echo "$response" | jq -e '
        (.errors // [])
        | map(
            (.code | tostring) as $code
            | (.message // "") as $message
            | ($code == "7003"
                or $code == "10000"
                or ($message | test("route|not found|404"; "i")))
        )
        | any
    ' >/dev/null 2>&1
}

cf_write_record() {
    local action="$1"
    local method="$2"
    local url="$3"
    local payload="$4"
    local response attempt max delay

    max="${CF_RETRY_ATTEMPTS:-5}"
    delay="${CF_RETRY_DELAY:-3}"
    (( max < 1 )) && max=1

    for ((attempt = 1; attempt <= max; attempt++)); do
        response="$(cf_headers -X "$method" "$url" --data "$payload")" || response=""
        if cf_success "$response"; then
            echo "$response"
            return 0
        fi

        if (( attempt < max )) && cf_retryable_response "$response"; then
            log "Cloudflare ${action} DNS 记录临时失败，${delay}s 后重试(${attempt}/${max}): $(cf_errors "$response")"
            sleep "$delay"
            continue
        fi

        echo "$response"
        return 0
    done
}

cf_ensure_zone_id() {
    local response success zone_info zone_id zone_name

    if [[ -n "${CF_ZONE_ID:-}" ]]; then
        return 0
    fi

    response="$(cf_headers -G "https://api.cloudflare.com/client/v4/zones" \
        --data-urlencode "per_page=100")" || {
        log "Cloudflare 自动识别 Zone ID 失败：无法读取 zones，请确认 API Token 有 Zone:Read 权限"
        return 1
    }

    success="$(echo "$response" | jq -r '.success // false')"
    [[ "$success" == "true" ]] || {
        log "Cloudflare 自动识别 Zone ID 失败: $(echo "$response" | jq -c '.errors // []')"
        return 1
    }

    zone_info="$(echo "$response" | jq -r --arg record "$CF_RECORD_NAME" '
        [.result[]
            | .name as $zone
            | select($record == $zone or ($record | endswith("." + $zone)))
            | {id, name}]
        | sort_by(.name | length)
        | last
        | if . then "\(.id)\t\(.name)" else "" end
    ')"

    if [[ -z "$zone_info" ]]; then
        log "Cloudflare 自动识别 Zone ID 失败：${CF_RECORD_NAME} 不在当前 Token 可访问的域名列表中"
        return 1
    fi

    zone_id="${zone_info%%$'\t'*}"
    zone_name="${zone_info#*$'\t'}"
    if [[ -z "$zone_id" || "$zone_id" == "$zone_name" ]]; then
        log "Cloudflare 自动识别 Zone ID 失败：返回数据格式异常"
        return 1
    fi

    CF_ZONE_ID="$zone_id"
    log "Cloudflare 自动识别 Zone ID 成功: ${zone_name}"
}

cf_get_record() {
    cf_ensure_zone_id || return 1
    cf_headers -G "https://api.cloudflare.com/client/v4/zones/${CF_ZONE_ID}/dns_records" \
        --data-urlencode "type=${CF_RECORD_TYPE}" \
        --data-urlencode "name=${CF_RECORD_NAME}"
}

cf_upsert_record() {
    local ip="$1"
    local proxied payload response success record_id old_ip

    proxied="$(normalize_bool "${CF_PROXIED}")"
    # cf_get_record is usually called through command substitution. Resolve the
    # Zone ID in the current shell first, otherwise an auto-detected CF_ZONE_ID
    # would be lost in the command-substitution subshell and create/update URLs
    # would become /zones//dns_records, which Cloudflare returns as 404.
    cf_ensure_zone_id || return 1
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
        response="$(cf_write_record "更新" "PUT" \
            "https://api.cloudflare.com/client/v4/zones/${CF_ZONE_ID}/dns_records/${record_id}" \
            "$payload")"
    else
        response="$(cf_write_record "创建" "POST" \
            "https://api.cloudflare.com/client/v4/zones/${CF_ZONE_ID}/dns_records" \
            "$payload")"
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
    log "换 IP curl 命令执行完成，等待 ${CHANGE_IP_WAIT_SECONDS}s 后继续"
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

    local ip now threshold cooldown ddns_enabled block_enabled
    ddns_enabled="$(normalize_bool "${DDNS_UPDATE_ENABLED}")"
    block_enabled="$(normalize_bool "${BLOCK_CHECK_ENABLED}")"
    ip="$(get_public_ip)" || fail "获取当前公网 IP 失败"
    LAST_IP="$ip"

    if [[ "$ddns_enabled" == "true" ]]; then
        cf_upsert_record "$ip" || fail "Cloudflare DDNS 更新失败"
    fi

    if [[ "$block_enabled" == "true" ]]; then
        if check_blocked "$ip"; then
            FAIL_COUNT=$((FAIL_COUNT + 1))
            threshold="${BLOCK_CHECK_FAIL_THRESHOLD:-1}"
            cooldown="${CHANGE_IP_COOLDOWN_SECONDS:-0}"
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
                        if [[ "$ddns_enabled" == "true" ]]; then
                            cf_upsert_record "$ip" || fail "换 IP 后 Cloudflare DDNS 更新失败"
                        fi
                    fi
                fi
            fi
        else
            if (( FAIL_COUNT > 0 )); then
                log "墙检测恢复正常，清零异常计数"
            fi
            FAIL_COUNT=0
        fi
    else
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
    echo "DDNS 更新: $(normalize_bool "${DDNS_UPDATE_ENABLED}")"
    if [[ "$(normalize_bool "${DDNS_UPDATE_ENABLED}")" == "true" ]]; then
        echo "Cloudflare Record: ${CF_RECORD_NAME} ${CF_RECORD_TYPE}"
        if [[ -n "${CF_ZONE_ID:-}" ]]; then
            echo "Zone ID: $(mask "${CF_ZONE_ID}")"
        else
            echo "Zone ID: 自动识别"
        fi
        echo "API Token: $(mask "${CF_API_TOKEN}")"
        echo "Proxied: $(normalize_bool "${CF_PROXIED}")"
    fi
    echo "检查间隔: ${CHECK_INTERVAL_MINUTES} 分钟"
    echo "墙检测: $(normalize_bool "${BLOCK_CHECK_ENABLED}")"
    if [[ -n "${BLOCK_CHECK_URL:-}" ]]; then
        echo "墙检测接口: ${BLOCK_CHECK_URL}"
    fi
    echo "异常计数: ${FAIL_COUNT:-0}/${BLOCK_CHECK_FAIL_THRESHOLD:-1}"
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
