#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

cur_dir=$(pwd)
V2NODE_REPO="${V2NODE_REPO:-jiasu9527/v2node}"
V2NODE_BRANCH="${V2NODE_BRANCH:-main}"

cache_bust_url() {
    local url="$1"
    local sep="?"
    [[ "$url" == *\?* ]] && sep="&"
    printf '%s%s_v2node_ts=%s' "$url" "$sep" "$(date +%s)"
}

download_stream() {
    local url="$1"
    curl -fsSL --retry 3 --retry-delay 2 \
        -H "Cache-Control: no-cache" \
        -H "Pragma: no-cache" \
        "$(cache_bust_url "$url")"
}

download_file() {
    local url="$1"
    local output="$2"
    curl -fsSL --retry 3 --retry-delay 2 \
        -H "Cache-Control: no-cache" \
        -H "Pragma: no-cache" \
        "$(cache_bust_url "$url")" \
        -o "$output"
}

run_remote_install() {
    if [[ -n "$1" ]]; then
        bash <(download_stream "https://raw.githubusercontent.com/${V2NODE_REPO}/${V2NODE_BRANCH}/script/install.sh") "$1"
    else
        bash <(download_stream "https://raw.githubusercontent.com/${V2NODE_REPO}/${V2NODE_BRANCH}/script/install.sh")
    fi
}

install_ddns_monitor_script() {
    local tmp_file="/tmp/v2node-ddns.$$.sh"
    local target="/usr/local/v2node/v2node-ddns"
    mkdir -p /usr/local/v2node
    if ! download_file "https://raw.githubusercontent.com/${V2NODE_REPO}/${V2NODE_BRANCH}/script/v2node-ddns.sh" "$tmp_file"; then
        rm -f "$tmp_file"
        echo -e "${red}下载 DDNS/墙检测脚本失败，请检查本机能否连接 Github${plain}"
        return 1
    fi
    chmod +x "$tmp_file"
    mv -f "$tmp_file" "$target"
    echo -e "${green}已安装 DDNS/墙检测脚本：${target}${plain}"
}

migrate_ddns_no_cooldown_config() {
    local file="/etc/v2node/ddns.env"
    [[ -f "$file" ]] || return 0

    local changed=false
    set_env_value() {
        local key="$1"
        local value="$2"
        if grep -q "^${key}=" "$file"; then
            sed -i "s#^${key}=.*#${key}=${value}#" "$file"
        else
            printf '%s=%s\n' "$key" "$value" >> "$file"
        fi
        changed=true
    }

    replace_default_value() {
        local key="$1"
        local old_value="$2"
        local new_value="$3"
        if grep -Eq "^${key}=${old_value}$" "$file"; then
            set_env_value "$key" "$new_value"
        fi
    }

    if grep -Eq "^BLOCK_CHECK_URL=['\"]?https://www\\.baidu\\.com/?['\"]?$" "$file"; then
        set_env_value "BLOCK_CHECK_URL" "'https://baidu.com/'"
    fi
    replace_default_value "BLOCK_CHECK_FAIL_THRESHOLD" "3" "1"
    replace_default_value "CHANGE_IP_WAIT_SECONDS" "60" "0"
    replace_default_value "CHANGE_IP_COOLDOWN_SECONDS" "1800" "0"

    if [[ "$changed" == "true" ]]; then
        echo -e "${green}已迁移墙检测配置：一次异常即换 IP，且不设置冷却${plain}"
    fi
}

# check root
[[ $EUID -ne 0 ]] && echo -e "${red}错误：${plain} 必须使用root用户运行此脚本！\n" && exit 1

# check os
if [[ -f /etc/redhat-release ]]; then
    release="centos"
elif cat /etc/issue | grep -Eqi "alpine"; then
    release="alpine"
elif cat /etc/issue | grep -Eqi "debian"; then
    release="debian"
elif cat /etc/issue | grep -Eqi "ubuntu"; then
    release="ubuntu"
elif cat /etc/issue | grep -Eqi "centos|red hat|redhat|rocky|alma|oracle linux"; then
    release="centos"
elif cat /proc/version | grep -Eqi "debian"; then
    release="debian"
elif cat /proc/version | grep -Eqi "ubuntu"; then
    release="ubuntu"
elif cat /proc/version | grep -Eqi "centos|red hat|redhat|rocky|alma|oracle linux"; then
    release="centos"
elif cat /proc/version | grep -Eqi "arch"; then
    release="arch"
else
    echo -e "${red}未检测到系统版本，请联系脚本作者！${plain}\n" && exit 1
fi

arch=$(uname -m)

if [[ $arch == "x86_64" || $arch == "x64" || $arch == "amd64" ]]; then
    arch="64"
elif [[ $arch == "aarch64" || $arch == "arm64" ]]; then
    arch="arm64-v8a"
elif [[ $arch == "s390x" ]]; then
    arch="s390x"
else
    arch="64"
    echo -e "${red}检测架构失败，使用默认架构: ${arch}${plain}"
fi

if [ "$(getconf WORD_BIT)" != '32' ] && [ "$(getconf LONG_BIT)" != '64' ] ; then
    echo "本软件不支持 32 位系统(x86)，请使用 64 位系统(x86_64)，如果检测有误，请联系作者"
    exit 2
fi

# os version
if [[ -f /etc/os-release ]]; then
    os_version=$(awk -F'[= ."]' '/VERSION_ID/{print $3}' /etc/os-release)
fi
if [[ -z "$os_version" && -f /etc/lsb-release ]]; then
    os_version=$(awk -F'[= ."]+' '/DISTRIB_RELEASE/{print $2}' /etc/lsb-release)
fi

if [[ x"${release}" == x"centos" ]]; then
    if [[ ${os_version} -le 6 ]]; then
        echo -e "${red}请使用 CentOS 7 或更高版本的系统！${plain}\n" && exit 1
    fi
    if [[ ${os_version} -eq 7 ]]; then
        echo -e "${red}注意： CentOS 7 无法使用hysteria1/2协议！${plain}\n"
    fi
elif [[ x"${release}" == x"ubuntu" ]]; then
    if [[ ${os_version} -lt 16 ]]; then
        echo -e "${red}请使用 Ubuntu 16 或更高版本的系统！${plain}\n" && exit 1
    fi
elif [[ x"${release}" == x"debian" ]]; then
    if [[ ${os_version} -lt 8 ]]; then
        echo -e "${red}请使用 Debian 8 或更高版本的系统！${plain}\n" && exit 1
    fi
fi

confirm() {
    if [[ $# > 1 ]]; then
        echo && read -rp "$1 [默认$2]: " temp
        if [[ x"${temp}" == x"" ]]; then
            temp=$2
        fi
    else
        read -rp "$1 [y/n]: " temp
    fi
    if [[ x"${temp}" == x"y" || x"${temp}" == x"Y" ]]; then
        return 0
    else
        return 1
    fi
}

confirm_restart() {
    confirm "是否重启v2node" "y"
    if [[ $? == 0 ]]; then
        restart
    else
        show_menu
    fi
}

before_show_menu() {
    echo && echo -n -e "${yellow}按回车返回主菜单: ${plain}" && read temp
    show_menu
}

install() {
    run_remote_install
    if [[ $? == 0 ]]; then
        if [[ $# == 0 ]]; then
            start
        else
            start 0
        fi
    fi
}

update() {
    if [[ $# == 0 ]]; then
        echo && echo -n -e "输入指定版本(默认最新版): " && read version
    else
        version=$2
    fi
    run_remote_install "$version"
    if [[ $? == 0 ]]; then
        echo -e "${green}更新完成，已自动重启 v2node，请使用 v2node log 查看运行日志${plain}"
        exit
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

config() {
    echo "v2node在修改配置后会自动尝试重启"
    vi /etc/v2node/config.json
    sleep 2
    restart
    check_status
    case $? in
        0)
            echo -e "v2node状态: ${green}已运行${plain}"
            ;;
        1)
            echo -e "检测到您未启动v2node或v2node自动重启失败，是否查看日志？[Y/n]" && echo
            read -e -rp "(默认: y):" yn
            [[ -z ${yn} ]] && yn="y"
            if [[ ${yn} == [Yy] ]]; then
               show_log
            fi
            ;;
        2)
            echo -e "v2node状态: ${red}未安装${plain}"
    esac
}

uninstall() {
    confirm "确定要卸载 v2node 吗?" "n"
    if [[ $? != 0 ]]; then
        if [[ $# == 0 ]]; then
            show_menu
        fi
        return 0
    fi
    disable_ddns_monitor 0 >/dev/null 2>&1 || true
    if [[ x"${release}" == x"alpine" ]]; then
        service v2node stop
        rc-update del v2node
        rm /etc/init.d/v2node -f
    else
        systemctl stop v2node
        systemctl disable v2node
        rm /etc/systemd/system/v2node.service -f
        systemctl daemon-reload
        systemctl reset-failed
    fi
    rm /etc/v2node/ -rf
    rm /usr/local/v2node/ -rf
    rm /var/lib/v2node/ddns.state -f
    rm /var/log/v2node-ddns.log -f

    echo ""
    echo -e "卸载成功，如果你想删除此脚本，则退出脚本后运行 ${green}rm /usr/bin/v2node -f${plain} 进行删除"
    echo ""

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

start() {
    check_status
    if [[ $? == 0 ]]; then
        echo ""
        echo -e "${green}v2node已运行，无需再次启动，如需重启请选择重启${plain}"
    else
        if [[ x"${release}" == x"alpine" ]]; then
            service v2node start
        else
            systemctl start v2node
        fi
        sleep 2
        check_status
        if [[ $? == 0 ]]; then
            echo -e "${green}v2node 启动成功，请使用 v2node log 查看运行日志${plain}"
        else
            echo -e "${red}v2node可能启动失败，请稍后使用 v2node log 查看日志信息${plain}"
        fi
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

stop() {
    if [[ x"${release}" == x"alpine" ]]; then
        service v2node stop
    else
        systemctl stop v2node
    fi
    sleep 2
    check_status
    if [[ $? == 1 ]]; then
        echo -e "${green}v2node 停止成功${plain}"
    else
        echo -e "${red}v2node停止失败，可能是因为停止时间超过了两秒，请稍后查看日志信息${plain}"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

restart() {
    if [[ x"${release}" == x"alpine" ]]; then
        service v2node restart
    else
        systemctl restart v2node
    fi
    sleep 2
    check_status
    if [[ $? == 0 ]]; then
        echo -e "${green}v2node 重启成功，请使用 v2node log 查看运行日志${plain}"
    else
        echo -e "${red}v2node可能启动失败，请稍后使用 v2node log 查看日志信息${plain}"
    fi
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

status() {
    if [[ x"${release}" == x"alpine" ]]; then
        service v2node status
    else
        systemctl status v2node --no-pager -l
    fi
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

enable() {
    if [[ x"${release}" == x"alpine" ]]; then
        rc-update add v2node
    else
        systemctl enable v2node
    fi
    if [[ $? == 0 ]]; then
        echo -e "${green}v2node 设置开机自启成功${plain}"
    else
        echo -e "${red}v2node 设置开机自启失败${plain}"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

disable() {
    if [[ x"${release}" == x"alpine" ]]; then
        rc-update del v2node
    else
        systemctl disable v2node
    fi
    if [[ $? == 0 ]]; then
        echo -e "${green}v2node 取消开机自启成功${plain}"
    else
        echo -e "${red}v2node 取消开机自启失败${plain}"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

show_log() {
    if [[ x"${release}" == x"alpine" ]]; then
        echo -e "${red}alpine系统暂不支持日志查看${plain}\n" && exit 1
    else
        journalctl -u v2node.service -e --no-pager -f
    fi
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

update_shell() {
    download_file "https://raw.githubusercontent.com/${V2NODE_REPO}/${V2NODE_BRANCH}/script/v2node.sh" /usr/bin/v2node
    if [[ $? != 0 ]]; then
        echo ""
        echo -e "${red}下载脚本失败，请检查本机能否连接 Github${plain}"
        before_show_menu
    else
        chmod +x /usr/bin/v2node
        if [[ -f /etc/v2node/ddns.env ]]; then
            migrate_ddns_no_cooldown_config || true
            install_ddns_monitor_script || true
        fi
        echo -e "${green}升级脚本成功，请重新运行脚本${plain}" && exit 0
    fi
}

# 0: running, 1: not running, 2: not installed
check_status() {
    if [[ ! -f /usr/local/v2node/v2node ]]; then
        return 2
    fi
    if [[ x"${release}" == x"alpine" ]]; then
        temp=$(service v2node status | awk '{print $3}')
        if [[ x"${temp}" == x"started" ]]; then
            return 0
        else
            return 1
        fi
    else
        temp=$(systemctl status v2node | grep Active | awk '{print $3}' | cut -d "(" -f2 | cut -d ")" -f1)
        if [[ x"${temp}" == x"running" ]]; then
            return 0
        else
            return 1
        fi
    fi
}

check_enabled() {
    if [[ x"${release}" == x"alpine" ]]; then
        temp=$(rc-update show | grep v2node)
        if [[ x"${temp}" == x"" ]]; then
            return 1
        else
            return 0
        fi
    else
        temp=$(systemctl is-enabled v2node)
        if [[ x"${temp}" == x"enabled" ]]; then
            return 0
        else
            return 1;
        fi
    fi
}

check_uninstall() {
    check_status
    if [[ $? != 2 ]]; then
        echo ""
        echo -e "${red}v2node已安装，请不要重复安装${plain}"
        if [[ $# == 0 ]]; then
            before_show_menu
        fi
        return 1
    else
        return 0
    fi
}

check_install() {
    check_status
    if [[ $? == 2 ]]; then
        echo ""
        echo -e "${red}请先安装v2node${plain}"
        if [[ $# == 0 ]]; then
            before_show_menu
        fi
        return 1
    else
        return 0
    fi
}

show_status() {
    check_status
    case $? in
        0)
            echo -e "v2node状态: ${green}已运行${plain}"
            show_enable_status
            ;;
        1)
            echo -e "v2node状态: ${yellow}未运行${plain}"
            show_enable_status
            ;;
        2)
            echo -e "v2node状态: ${red}未安装${plain}"
    esac
}

show_enable_status() {
    check_enabled
    if [[ $? == 0 ]]; then
        echo -e "是否开机自启: ${green}是${plain}"
    else
        echo -e "是否开机自启: ${red}否${plain}"
    fi
}

show_v2node_version() {
    echo -n "v2node 版本："
    /usr/local/v2node/v2node version
    echo ""
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

generate_v2node_config() {
        local api_host="$1"
        local node_id="$2"
        local api_key="$3"

        mkdir -p /etc/v2node >/dev/null 2>&1
        cat > /etc/v2node/config.json <<EOF
{
    "Log": {
        "Level": "warning",
        "Output": "",
        "Access": "none"
    },
    "Nodes": [
        {
            "ApiHost": "${api_host}",
            "NodeID": ${node_id},
            "ApiKey": "${api_key}",
            "Timeout": 15
        }
    ]
}
EOF
        echo -e "${green}V2node 配置文件生成完成,正在重新启动服务${plain}"
        if [[ x"${release}" == x"alpine" ]]; then
            service v2node restart
        else
            systemctl restart v2node
        fi
        sleep 2
        check_status
        echo -e ""
        if [[ $? == 0 ]]; then
            echo -e "${green}v2node 重启成功${plain}"
        else
            echo -e "${red}v2node 可能启动失败，请使用 v2node log 查看日志信息${plain}"
        fi
}


generate_config_file() {
    # 交互式收集参数，提供示例默认值
    read -rp "面板API地址[格式: https://example.com/]: " api_host
    api_host=${api_host:-https://example.com/}
    read -rp "节点ID: " node_id
    node_id=${node_id:-1}
    read -rp "节点通讯密钥: " api_key

    # 生成配置文件（覆盖可能从包中复制的模板）
    generate_v2node_config "$api_host" "$node_id" "$api_key"
}

env_quote() {
    local value="$1"
    printf "'%s'" "$(printf "%s" "$value" | sed "s/'/'\\\\''/g")"
}

ensure_ddns_dependencies() {
    local missing=()
    for cmd in curl jq; do
        command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
    done
    [[ ${#missing[@]} -eq 0 ]] && return 0

    echo -e "${yellow}安装 DDNS 所需依赖: ${missing[*]}${plain}"
    if [[ x"${release}" == x"centos" ]]; then
        yum install -y epel-release >/dev/null 2>&1 || true
        yum install -y curl jq >/dev/null 2>&1
    elif [[ x"${release}" == x"alpine" ]]; then
        apk add --no-cache curl jq dcron >/dev/null 2>&1
    elif [[ x"${release}" == x"debian" || x"${release}" == x"ubuntu" ]]; then
        apt-get update -y >/dev/null 2>&1
        DEBIAN_FRONTEND=noninteractive apt-get install -y curl jq >/dev/null 2>&1
    elif [[ x"${release}" == x"arch" ]]; then
        pacman -Sy --noconfirm --needed curl jq >/dev/null 2>&1
    fi
}

normalize_minutes() {
    local value="$1"
    [[ "$value" =~ ^[0-9]+$ ]] || value=1
    (( value < 1 )) && value=1
    (( value > 59 )) && value=59
    echo "$value"
}

write_ddns_config() {
    local cf_token="$1"
    local cf_zone_id="$2"
    local cf_record_name="$3"
    local cf_record_type="$4"
    local cf_ttl="$5"
    local cf_proxied="$6"
    local interval="$7"
    local ddns_enabled="$8"
    local block_enabled="$9"
    local block_url="${10}"
    local block_keyword="${11}"
    local block_timeout="${12}"
    local block_threshold="${13}"
    local change_cmd="${14}"
    local change_wait="${15}"
    local change_cooldown="${16}"

    mkdir -p /etc/v2node
    umask 077
    cat > /etc/v2node/ddns.env <<EOF
CF_API_TOKEN=$(env_quote "$cf_token")
CF_ZONE_ID=$(env_quote "$cf_zone_id")
CF_RECORD_NAME=$(env_quote "$cf_record_name")
CF_RECORD_TYPE=$(env_quote "$cf_record_type")
CF_TTL=${cf_ttl}
CF_PROXIED=${cf_proxied}
CHECK_INTERVAL_MINUTES=${interval}
DDNS_UPDATE_ENABLED=${ddns_enabled}
BLOCK_CHECK_ENABLED=${block_enabled}
BLOCK_CHECK_URL=$(env_quote "$block_url")
BLOCK_CHECK_BLOCKED_KEYWORD=$(env_quote "$block_keyword")
BLOCK_CHECK_TIMEOUT=${block_timeout}
BLOCK_CHECK_FAIL_THRESHOLD=${block_threshold}
CHANGE_IP_CURL_CMD=$(env_quote "$change_cmd")
CHANGE_IP_WAIT_SECONDS=${change_wait}
CHANGE_IP_COOLDOWN_SECONDS=${change_cooldown}
EOF
    chmod 600 /etc/v2node/ddns.env
}

load_ddns_interval() {
    local interval=1
    if [[ -f /etc/v2node/ddns.env ]]; then
        # shellcheck source=/dev/null
        . /etc/v2node/ddns.env
        interval="${CHECK_INTERVAL_MINUTES:-1}"
    fi
    normalize_minutes "$interval"
}

install_ddns_timer() {
    local interval
    interval="$(load_ddns_interval)"

    if [[ ! -x /usr/local/v2node/v2node-ddns ]]; then
        install_ddns_monitor_script || return 1
    fi

    if [[ x"${release}" == x"alpine" ]]; then
        ensure_ddns_dependencies
        (crontab -l 2>/dev/null | grep -v '/usr/local/v2node/v2node-ddns run' || true; \
            echo "*/${interval} * * * * /usr/local/v2node/v2node-ddns run >/dev/null 2>&1") | crontab -
        service crond start >/dev/null 2>&1 || true
        rc-update add crond default >/dev/null 2>&1 || true
        echo -e "${green}DDNS/墙检测 cron 已启用，每 ${interval} 分钟执行一次${plain}"
    else
        cat > /etc/systemd/system/v2node-ddns.service <<EOF
[Unit]
Description=v2node Cloudflare DDNS and GFW block checker
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/v2node/v2node-ddns run
EOF
        cat > /etc/systemd/system/v2node-ddns.timer <<EOF
[Unit]
Description=Run v2node DDNS/GFW checker periodically

[Timer]
OnBootSec=1min
OnUnitActiveSec=${interval}min
AccuracySec=30s
Unit=v2node-ddns.service

[Install]
WantedBy=timers.target
EOF
        systemctl daemon-reload
        systemctl enable --now v2node-ddns.timer
        echo -e "${green}DDNS/墙检测 systemd timer 已启用，每 ${interval} 分钟执行一次${plain}"
    fi
}

disable_ddns_monitor() {
    if [[ x"${release}" == x"alpine" ]]; then
        if command -v crontab >/dev/null 2>&1; then
            (crontab -l 2>/dev/null | grep -v '/usr/local/v2node/v2node-ddns run' || true) | crontab -
        fi
    else
        systemctl disable --now v2node-ddns.timer >/dev/null 2>&1 || true
        rm -f /etc/systemd/system/v2node-ddns.service /etc/systemd/system/v2node-ddns.timer
        systemctl daemon-reload >/dev/null 2>&1 || true
        systemctl reset-failed v2node-ddns.service v2node-ddns.timer >/dev/null 2>&1 || true
    fi
    echo -e "${green}DDNS/墙检测已停用${plain}"
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

configure_ddns_monitor() {
    ensure_ddns_dependencies

    echo -e "${yellow}Cloudflare DDNS/墙检测自动换IP配置${plain}"
    echo "说明：DDNS 使用 Cloudflare API；墙检测默认访问 https://baidu.com/。"
    echo "检测接口支持占位符：{ip} 当前公网IP，{domain} DNS记录名。"
    echo ""

    local ddns_enabled=false
    local block_enabled=false
    local cf_token=""
    local cf_zone_id=""
    local cf_record_name=""
    local cf_record_type="A"
    local cf_ttl=1
    local cf_proxied=false
    local interval
    local block_url=""
    local block_keyword=""
    local block_timeout=10
    local block_threshold=1
    local change_cmd=""
    local change_wait=0
    local change_cooldown=0

    read -rp "是否启用 DDNS 解析更新？[y/N]: " ddns_input
    if [[ "$ddns_input" =~ ^[Yy]$ ]]; then
        ddns_enabled=true
        read -rsp "Cloudflare API Token: " cf_token
        echo ""
        read -rp "DNS记录完整域名(例如 hk.example.com): " cf_record_name
        read -rp "记录类型[A/AAAA，默认A]: " cf_record_type
        cf_record_type=${cf_record_type:-A}
        cf_record_type=$(echo "$cf_record_type" | tr '[:lower:]' '[:upper:]')
        [[ "$cf_record_type" != "A" && "$cf_record_type" != "AAAA" ]] && cf_record_type="A"
        read -rp "TTL[默认1=自动]: " cf_ttl
        cf_ttl=${cf_ttl:-1}
        [[ "$cf_ttl" =~ ^[0-9]+$ ]] || cf_ttl=1
        read -rp "是否开启Cloudflare代理橙云？[y/N]: " proxied_input
        if [[ "$proxied_input" =~ ^[Yy]$ ]]; then
            cf_proxied=true
        fi
    fi

    read -rp "检查间隔分钟[默认1，最大59]: " interval
    interval=$(normalize_minutes "${interval:-1}")

    read -rp "是否启用被墙检测自动换IP？[y/N]: " block_input
    if [[ "$block_input" =~ ^[Yy]$ ]]; then
        block_enabled=true
        read -rp "墙检测接口URL[默认 https://baidu.com/，支持 {ip}/{domain}]: " block_url
        block_url=${block_url:-https://baidu.com/}
        read -rp "返回内容包含哪个关键词表示已被墙[留空=接口curl失败才算异常]: " block_keyword
        read -rp "检测超时时间秒[默认10]: " block_timeout
        block_timeout=${block_timeout:-10}
        [[ "$block_timeout" =~ ^[0-9]+$ ]] || block_timeout=10
        read -rp "连续异常多少次后换IP[默认1]: " block_threshold
        block_threshold=${block_threshold:-1}
        [[ "$block_threshold" =~ ^[0-9]+$ ]] || block_threshold=1
        read -rp "换IP curl完整命令(例如 curl -fsS 'https://api.xxx/change?token=xxx'): " change_cmd
        read -rp "换IP后等待秒数[默认0]: " change_wait
        change_wait=${change_wait:-0}
        [[ "$change_wait" =~ ^[0-9]+$ ]] || change_wait=0
        read -rp "换IP冷却秒数[默认0]: " change_cooldown
        change_cooldown=${change_cooldown:-0}
        [[ "$change_cooldown" =~ ^[0-9]+$ ]] || change_cooldown=0
    fi

    if [[ "$ddns_enabled" != "true" && "$block_enabled" != "true" ]]; then
        echo -e "${yellow}未启用 DDNS 或被墙检测，已取消配置${plain}"
        if [[ $# == 0 ]]; then
            before_show_menu
        fi
        return 0
    fi

    if [[ "$ddns_enabled" == "true" ]]; then
        if [[ -z "$cf_token" || -z "$cf_record_name" ]]; then
            echo -e "${red}Cloudflare API Token / DNS记录名不能为空${plain}"
            if [[ $# == 0 ]]; then
                before_show_menu
            fi
            return 1
        fi
    fi

    write_ddns_config "$cf_token" "$cf_zone_id" "$cf_record_name" "$cf_record_type" "$cf_ttl" "$cf_proxied" \
        "$interval" "$ddns_enabled" "$block_enabled" "$block_url" "$block_keyword" "$block_timeout" "$block_threshold" \
        "$change_cmd" "$change_wait" "$change_cooldown"

    install_ddns_monitor_script || return 1
    install_ddns_timer || return 1
    echo -e "${green}DDNS/墙检测配置已写入 /etc/v2node/ddns.env${plain}"
    echo -e "${yellow}正在执行一次 DDNS 检测，请稍候...${plain}"
    /usr/local/v2node/v2node-ddns run || true

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

bool_value() {
    case "$(echo "${1:-false}" | tr '[:upper:]' '[:lower:]')" in
        1|true|yes|y|on) echo "true" ;;
        *) echo "false" ;;
    esac
}

configure_cloudflare_ddns() {
    ensure_ddns_dependencies

    local ddns_enabled=true
    local block_enabled=false
    local cf_token=""
    local cf_zone_id=""
    local cf_record_name=""
    local cf_record_type="A"
    local cf_ttl=1
    local cf_proxied=false
    local interval=1
    local block_url=""
    local block_keyword=""
    local block_timeout=10
    local block_threshold=1
    local change_cmd=""
    local change_wait=0
    local change_cooldown=0

    if [[ -f /etc/v2node/ddns.env ]]; then
        # shellcheck source=/dev/null
        . /etc/v2node/ddns.env
        cf_token="${CF_API_TOKEN:-}"
        cf_zone_id="${CF_ZONE_ID:-}"
        cf_record_name="${CF_RECORD_NAME:-}"
        cf_record_type="${CF_RECORD_TYPE:-A}"
        cf_ttl="${CF_TTL:-1}"
        cf_proxied="$(bool_value "${CF_PROXIED:-false}")"
        interval="${CHECK_INTERVAL_MINUTES:-1}"
        block_enabled="$(bool_value "${BLOCK_CHECK_ENABLED:-false}")"
        block_url="${BLOCK_CHECK_URL:-}"
        block_keyword="${BLOCK_CHECK_BLOCKED_KEYWORD:-}"
        block_timeout="${BLOCK_CHECK_TIMEOUT:-10}"
        block_threshold="${BLOCK_CHECK_FAIL_THRESHOLD:-1}"
        change_cmd="${CHANGE_IP_CURL_CMD:-}"
        change_wait="${CHANGE_IP_WAIT_SECONDS:-0}"
        change_cooldown="${CHANGE_IP_COOLDOWN_SECONDS:-0}"
    fi

    echo -e "${yellow}Cloudflare DDNS 配置${plain}"
    echo "留空表示沿用已有值；首次配置时 Cloudflare API Token 和 DNS记录名不能为空。"
    echo ""

    local input
    read -rsp "Cloudflare API Token${cf_token:+[留空沿用已有]}: " input
    echo ""
    [[ -n "$input" ]] && cf_token="$input"
    read -rp "Cloudflare Zone ID[可留空自动识别]: " input
    [[ -n "$input" ]] && cf_zone_id="$input"
    read -rp "DNS记录完整域名${cf_record_name:+[当前: $cf_record_name]}: " input
    [[ -n "$input" ]] && cf_record_name="$input"
    read -rp "记录类型[A/AAAA，默认${cf_record_type}]: " input
    [[ -n "$input" ]] && cf_record_type="$input"
    cf_record_type=$(echo "${cf_record_type:-A}" | tr '[:lower:]' '[:upper:]')
    [[ "$cf_record_type" != "A" && "$cf_record_type" != "AAAA" ]] && cf_record_type="A"
    read -rp "TTL[默认${cf_ttl}，1=自动]: " input
    [[ -n "$input" ]] && cf_ttl="$input"
    [[ "$cf_ttl" =~ ^[0-9]+$ ]] || cf_ttl=1
    read -rp "是否开启Cloudflare代理橙云？[y/N，当前${cf_proxied}]: " input
    if [[ "$input" =~ ^[Yy]$ ]]; then
        cf_proxied=true
    elif [[ "$input" =~ ^[Nn]$ ]]; then
        cf_proxied=false
    fi
    read -rp "检查间隔分钟[默认${interval}，最大59]: " input
    [[ -n "$input" ]] && interval="$input"
    interval=$(normalize_minutes "${interval:-1}")

    if [[ -z "$cf_token" || -z "$cf_record_name" ]]; then
        echo -e "${red}Cloudflare API Token / DNS记录名不能为空${plain}"
        if [[ $# == 0 ]]; then
            before_show_menu
        fi
        return 1
    fi

    write_ddns_config "$cf_token" "$cf_zone_id" "$cf_record_name" "$cf_record_type" "$cf_ttl" "$cf_proxied" \
        "$interval" "$ddns_enabled" "$block_enabled" "$block_url" "$block_keyword" "$block_timeout" "$block_threshold" \
        "$change_cmd" "$change_wait" "$change_cooldown"

    install_ddns_monitor_script || return 1
    install_ddns_timer || return 1
    echo -e "${green}Cloudflare DDNS 配置已写入 /etc/v2node/ddns.env${plain}"
    /usr/local/v2node/v2node-ddns run || true

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

configure_block_check() {
    ensure_ddns_dependencies

    local ddns_enabled=false
    local block_enabled=true
    local cf_token=""
    local cf_zone_id=""
    local cf_record_name=""
    local cf_record_type="A"
    local cf_ttl=1
    local cf_proxied=false
    local interval=1
    local block_url="https://baidu.com/"
    local block_keyword=""
    local block_timeout=10
    local block_threshold=1
    local change_cmd=""
    local change_wait=0
    local change_cooldown=0

    if [[ -f /etc/v2node/ddns.env ]]; then
        # shellcheck source=/dev/null
        . /etc/v2node/ddns.env
        ddns_enabled="$(bool_value "${DDNS_UPDATE_ENABLED:-false}")"
        cf_token="${CF_API_TOKEN:-}"
        cf_zone_id="${CF_ZONE_ID:-}"
        cf_record_name="${CF_RECORD_NAME:-}"
        cf_record_type="${CF_RECORD_TYPE:-A}"
        cf_ttl="${CF_TTL:-1}"
        cf_proxied="$(bool_value "${CF_PROXIED:-false}")"
        interval="${CHECK_INTERVAL_MINUTES:-1}"
        block_url="${BLOCK_CHECK_URL:-https://baidu.com/}"
        block_keyword="${BLOCK_CHECK_BLOCKED_KEYWORD:-}"
        block_timeout="${BLOCK_CHECK_TIMEOUT:-10}"
        block_threshold="${BLOCK_CHECK_FAIL_THRESHOLD:-1}"
        change_cmd="${CHANGE_IP_CURL_CMD:-}"
        change_wait="${CHANGE_IP_WAIT_SECONDS:-0}"
        change_cooldown="${CHANGE_IP_COOLDOWN_SECONDS:-0}"
    fi

    echo -e "${yellow}被墙检测 / 自动换 IP 配置${plain}"
    echo "默认访问 https://baidu.com/，curl 失败就累计一次异常。"
    echo ""

    local input
    read -rp "检查间隔分钟[默认${interval}，最大59]: " input
    [[ -n "$input" ]] && interval="$input"
    interval=$(normalize_minutes "${interval:-1}")
    read -rp "墙检测接口URL[默认${block_url}，支持 {ip}/{domain}]: " input
    [[ -n "$input" ]] && block_url="$input"
    block_url=${block_url:-https://baidu.com/}
    read -rp "返回内容包含哪个关键词表示已被墙[留空=接口curl失败才算异常]${block_keyword:+[当前: $block_keyword]}: " input
    [[ -n "$input" ]] && block_keyword="$input"
    read -rp "检测超时时间秒[默认${block_timeout}]: " input
    [[ -n "$input" ]] && block_timeout="$input"
    [[ "$block_timeout" =~ ^[0-9]+$ ]] || block_timeout=10
    read -rp "连续异常多少次后换IP[默认${block_threshold}]: " input
    [[ -n "$input" ]] && block_threshold="$input"
    [[ "$block_threshold" =~ ^[0-9]+$ ]] || block_threshold=1
    read -rp "换IP curl完整命令${change_cmd:+[留空沿用已有]}: " input
    [[ -n "$input" ]] && change_cmd="$input"
    read -rp "换IP后等待秒数[默认${change_wait}]: " input
    [[ -n "$input" ]] && change_wait="$input"
    [[ "$change_wait" =~ ^[0-9]+$ ]] || change_wait=0
    read -rp "换IP冷却秒数[默认${change_cooldown}]: " input
    [[ -n "$input" ]] && change_cooldown="$input"
    [[ "$change_cooldown" =~ ^[0-9]+$ ]] || change_cooldown=0

    write_ddns_config "$cf_token" "$cf_zone_id" "$cf_record_name" "$cf_record_type" "$cf_ttl" "$cf_proxied" \
        "$interval" "$ddns_enabled" "$block_enabled" "$block_url" "$block_keyword" "$block_timeout" "$block_threshold" \
        "$change_cmd" "$change_wait" "$change_cooldown"

    install_ddns_monitor_script || return 1
    install_ddns_timer || return 1
    echo -e "${green}被墙检测配置已写入 /etc/v2node/ddns.env${plain}"
    /usr/local/v2node/v2node-ddns run || true

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

configure_ddns_monitor_from_args() {
    local cf_token=""
    local cf_zone_id=""
    local cf_record_name=""
    local cf_record_type="A"
    local cf_ttl="1"
    local cf_proxied="false"
    local interval="1"
    local block_url=""
    local block_keyword=""
    local block_timeout="10"
    local block_threshold="1"
    local change_cmd=""
    local change_wait="0"
    local change_cooldown="0"
    local ddns_enabled="false"
    local block_enabled="false"

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --enable-ddns) ddns_enabled="true"; shift ;;
            --enable-block-check) block_enabled="true"; shift ;;
            --cf-token) cf_token="$2"; ddns_enabled="true"; shift 2 ;;
            --cf-zone-id) cf_zone_id="$2"; ddns_enabled="true"; shift 2 ;;
            --cf-record) cf_record_name="$2"; ddns_enabled="true"; shift 2 ;;
            --cf-record-type) cf_record_type="$2"; ddns_enabled="true"; shift 2 ;;
            --cf-ttl) cf_ttl="$2"; ddns_enabled="true"; shift 2 ;;
            --cf-proxied) cf_proxied="$2"; ddns_enabled="true"; shift 2 ;;
            --ddns-interval) interval="$2"; shift 2 ;;
            --block-check-url) block_url="$2"; block_enabled="true"; shift 2 ;;
            --block-check-keyword) block_keyword="$2"; block_enabled="true"; shift 2 ;;
            --block-check-timeout) block_timeout="$2"; block_enabled="true"; shift 2 ;;
            --block-check-threshold) block_threshold="$2"; block_enabled="true"; shift 2 ;;
            --change-ip-curl) change_cmd="$2"; block_enabled="true"; shift 2 ;;
            --change-ip-wait) change_wait="$2"; block_enabled="true"; shift 2 ;;
            --change-ip-cooldown) change_cooldown="$2"; block_enabled="true"; shift 2 ;;
            -h|--help)
                echo "用法: v2node ddns-set [--enable-ddns --cf-token TOKEN --cf-record DOMAIN] [--enable-block-check --block-check-url URL --change-ip-curl CMD]"
                return 0 ;;
            *)
                echo -e "${red}未知 DDNS 参数: $1${plain}"
                return 1 ;;
        esac
    done

    cf_record_type=$(echo "${cf_record_type:-A}" | tr '[:lower:]' '[:upper:]')
    [[ "$cf_record_type" != "A" && "$cf_record_type" != "AAAA" ]] && cf_record_type="A"
    [[ "$cf_ttl" =~ ^[0-9]+$ ]] || cf_ttl=1
    interval=$(normalize_minutes "${interval:-1}")
    [[ "$block_timeout" =~ ^[0-9]+$ ]] || block_timeout=10
    [[ "$block_threshold" =~ ^[0-9]+$ ]] || block_threshold=1
    [[ "$change_wait" =~ ^[0-9]+$ ]] || change_wait=0
    [[ "$change_cooldown" =~ ^[0-9]+$ ]] || change_cooldown=0
    if [[ "$cf_proxied" =~ ^([Tt][Rr][Uu][Ee]|1|[Yy]|[Yy][Ee][Ss])$ ]]; then
        cf_proxied=true
    else
        cf_proxied=false
    fi

    if [[ "$ddns_enabled" == "true" ]]; then
        if [[ -z "$cf_token" || -z "$cf_record_name" ]]; then
            echo -e "${red}缺少 --cf-token / --cf-record${plain}"
            return 1
        fi
    fi
    if [[ "$block_enabled" == "true" && -z "$block_url" ]]; then
        block_url="https://baidu.com/"
    fi

    if [[ "$ddns_enabled" != "true" && "$block_enabled" != "true" ]]; then
        echo -e "${yellow}未启用 DDNS 或被墙检测，跳过配置${plain}"
        return 0
    fi

    ensure_ddns_dependencies
    write_ddns_config "$cf_token" "$cf_zone_id" "$cf_record_name" "$cf_record_type" "$cf_ttl" "$cf_proxied" \
        "$interval" "$ddns_enabled" "$block_enabled" "$block_url" "$block_keyword" "$block_timeout" "$block_threshold" \
        "$change_cmd" "$change_wait" "$change_cooldown"

    install_ddns_monitor_script || return 1
    install_ddns_timer || return 1
    echo -e "${green}DDNS/墙检测配置已写入 /etc/v2node/ddns.env${plain}"
    /usr/local/v2node/v2node-ddns run || true
}

run_ddns_once() {
    if [[ ! -x /usr/local/v2node/v2node-ddns ]]; then
        install_ddns_monitor_script || return 1
    fi
    /usr/local/v2node/v2node-ddns run
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

run_block_check_once() {
    if [[ ! -x /usr/local/v2node/v2node-ddns ]]; then
        install_ddns_monitor_script || return 1
    fi
    /usr/local/v2node/v2node-ddns block-check-run
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

show_ddns_status() {
    if [[ ! -x /usr/local/v2node/v2node-ddns ]]; then
        install_ddns_monitor_script >/dev/null 2>&1 || true
    fi
    if [[ -x /usr/local/v2node/v2node-ddns ]]; then
        /usr/local/v2node/v2node-ddns status || true
    else
        echo -e "${red}DDNS/墙检测脚本未安装${plain}"
    fi
    if [[ x"${release}" != x"alpine" ]]; then
        systemctl status v2node-ddns.timer --no-pager -l 2>/dev/null || true
    fi
    if [[ -f /var/log/v2node-ddns.log ]]; then
        echo "最近日志："
        tail -n 20 /var/log/v2node-ddns.log
    fi
    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}



show_external_status() {
    local config_dir="${V2NODE_EXTERNAL_CONFIG_DIR:-/etc/v2node}"

    echo -e "${yellow}Juicity/Mieru 外部协议状态${plain}"
    echo "配置目录: ${config_dir}"
    echo ""

    echo "依赖命令:"
    if command -v juicity-server >/dev/null 2>&1; then
        echo -e "  juicity-server: ${green}$(command -v juicity-server)${plain}"
    else
        echo -e "  juicity-server: ${red}未安装/不在PATH${plain}"
    fi
    if command -v mita >/dev/null 2>&1; then
        echo -e "  mita: ${green}$(command -v mita)${plain}"
    else
        echo -e "  mita: ${red}未安装/不在PATH${plain}"
    fi
    echo ""

    echo "外部协议配置文件:"
    shopt -s nullglob
    local configs=("${config_dir}"/external-juicity-*.json "${config_dir}"/external-mieru-*.json)
    shopt -u nullglob
    if [[ ${#configs[@]} -eq 0 ]]; then
        echo -e "  ${yellow}未发现 external-juicity-*.json / external-mieru-*.json${plain}"
    else
        local cfg
        for cfg in "${configs[@]}"; do
            echo "  ${cfg}"
            if [[ -r "$cfg" ]]; then
                if command -v jq >/dev/null 2>&1; then
                    jq -r 'if has("listen") then "    listen=" + (.listen|tostring) elif has("portBindings") then "    portBindings=" + (.portBindings|tostring) else "    未识别端口字段" end' "$cfg" 2>/dev/null || true
                else
                    grep -E '"listen"|"port"|"protocol"|"v2node_observer_log"|"observer_log"|"access_log"' "$cfg" | sed 's/^/    /' || true
                fi
            else
                echo -e "    ${red}不可读${plain}"
            fi
        done
    fi
    echo ""

    echo "外部协议进程:"
    if pgrep -af "juicity-server|mita" >/dev/null 2>&1; then
        pgrep -af "juicity-server|mita" | sed 's/^/  /'
    else
        echo -e "  ${yellow}未发现 juicity-server / mita 进程${plain}"
    fi
    echo ""

    echo "监听端口(可能需要 root 权限显示进程名):"
    if command -v ss >/dev/null 2>&1; then
        ss -lntup 2>/dev/null | grep -E 'juicity|mita|v2node|LISTEN' || true
    elif command -v netstat >/dev/null 2>&1; then
        netstat -lntup 2>/dev/null | grep -E 'juicity|mita|v2node|LISTEN' || true
    else
        echo -e "  ${yellow}未安装 ss/netstat，无法查看监听端口${plain}"
    fi
    echo ""

    if command -v mita >/dev/null 2>&1; then
        echo "Mieru metrics 预览:"
        mita get metrics 2>&1 | head -n 40 || true
        echo ""
    fi

    show_observer_status 0

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

show_observer_status() {
    local config_dir="${V2NODE_EXTERNAL_CONFIG_DIR:-/etc/v2node}"
    local found=false

    echo -e "${yellow}Juicity observer 状态${plain}"
    echo "配置目录: ${config_dir}"
    echo ""

    shopt -s nullglob
    local configs=("${config_dir}"/external-juicity-*.json)
    local logs=("${config_dir}"/external-juicity-*.observe.jsonl)
    shopt -u nullglob

    if [[ ${#configs[@]} -eq 0 && ${#logs[@]} -eq 0 ]]; then
        echo -e "${yellow}未发现 Juicity 外部协议配置或 observer 日志${plain}"
        echo "请确认面板节点协议为 juicity，且 v2node 已完成一次拉取/重载。"
    fi

    for cfg in "${configs[@]}"; do
        found=true
        echo "配置文件: ${cfg}"
        if [[ -r "$cfg" ]]; then
            if command -v jq >/dev/null 2>&1; then
                local cfg_log
                cfg_log="$(jq -r '.v2node_observer_log // .observer_log // .access_log // empty' "$cfg" 2>/dev/null || true)"
                if [[ -n "$cfg_log" ]]; then
                    echo "observer日志路径: ${cfg_log}"
                else
                    echo -e "observer日志路径: ${red}未配置 v2node_observer_log / observer_log / access_log${plain}"
                fi
            else
                echo "observer相关配置:"
                grep -E 'v2node_observer_log|observer_log|access_log' "$cfg" || echo -e "${yellow}未检测到 observer 日志字段；安装 jq 后可查看解析结果${plain}"
            fi
        else
            echo -e "${red}配置文件不可读${plain}"
        fi
        echo ""
    done

    for log_file in "${logs[@]}"; do
        found=true
        echo "日志文件: ${log_file}"
        if [[ -e "$log_file" ]]; then
            local size lines mtime
            size=$(wc -c < "$log_file" 2>/dev/null | tr -d ' ')
            lines=$(wc -l < "$log_file" 2>/dev/null | tr -d ' ')
            if stat -c %y "$log_file" >/dev/null 2>&1; then
                mtime=$(stat -c %y "$log_file")
            else
                mtime=$(stat -f "%Sm" "$log_file" 2>/dev/null || echo "unknown")
            fi
            echo "大小: ${size:-0} bytes, 行数: ${lines:-0}, 更新时间: ${mtime}"
            if [[ "${size:-0}" -gt 0 ]]; then
                echo "最后20行:"
                tail -n 20 "$log_file"
            else
                echo -e "${yellow}日志为空：patched juicity-server 可能尚未写入 observer 事件${plain}"
            fi
        else
            echo -e "${red}日志文件不存在${plain}"
        fi
        echo ""
    done

    if [[ "$found" != "true" ]]; then
        echo "常见原因："
        echo "1. 节点还没被 v2node 拉取到；"
        echo "2. 面板没有把该节点下发为 juicity external_protocol；"
        echo "3. 正在运行的是原版 juicity-server，它不会写 observer JSONL。"
    fi

    if [[ $# == 0 ]]; then
        before_show_menu
    fi
}

# 放开防火墙端口
open_ports() {
    systemctl stop firewalld.service 2>/dev/null
    systemctl disable firewalld.service 2>/dev/null
    setenforce 0 2>/dev/null
    ufw disable 2>/dev/null
    iptables -P INPUT ACCEPT 2>/dev/null
    iptables -P FORWARD ACCEPT 2>/dev/null
    iptables -P OUTPUT ACCEPT 2>/dev/null
    iptables -t nat -F 2>/dev/null
    iptables -t mangle -F 2>/dev/null
    iptables -F 2>/dev/null
    iptables -X 2>/dev/null
    netfilter-persistent save 2>/dev/null
    echo -e "${green}放开防火墙端口成功！${plain}"
}

show_usage() {
    echo "v2node 管理脚本使用方法: "
    echo "------------------------------------------"
    echo "v2node              - 显示管理菜单 (功能更多)"
    echo "v2node start        - 启动 v2node"
    echo "v2node stop         - 停止 v2node"
    echo "v2node restart      - 重启 v2node"
    echo "v2node status       - 查看 v2node 状态"
    echo "v2node enable       - 设置 v2node 开机自启"
    echo "v2node disable      - 取消 v2node 开机自启"
    echo "v2node log          - 查看 v2node 日志"
    echo "v2node x25519       - 生成 x25519 密钥"
    echo "v2node generate     - 生成 v2node 配置文件"
    echo "v2node ddns         - 配置 Cloudflare DDNS"
    echo "v2node block-check  - 配置被墙检测/自动换IP"
    echo "v2node ddns-all     - 配置 Cloudflare DDNS/墙检测自动换IP"
    echo "v2node ddns-set     - 使用命令行参数配置 DDNS/墙检测"
    echo "v2node ddns-run     - 立即执行一次 DDNS/墙检测"
    echo "v2node block-check-run - 只执行一次被墙检测/自动换IP"
    echo "v2node ddns-status  - 查看 DDNS/墙检测状态"
    echo "v2node ddns-disable - 停用 DDNS/墙检测定时任务"
    echo "v2node observer-status - 查看 Juicity observer 采集状态"
    echo "v2node external-status - 查看 Juicity/Mieru 外部协议状态"
    echo "v2node update       - 更新 v2node"
    echo "v2node update x.x.x - 安装 v2node 指定版本"
    echo "v2node install      - 安装 v2node"
    echo "v2node uninstall    - 卸载 v2node"
    echo "v2node version      - 查看 v2node 版本"
    echo "------------------------------------------"
}

show_menu() {
    echo -e "
  ${green}v2node 后端管理脚本，${plain}${red}不适用于docker${plain}
--- https://github.com/${V2NODE_REPO} ---
  ${green}0.${plain} 修改配置
————————————————
  ${green}1.${plain} 安装 v2node
  ${green}2.${plain} 更新 v2node
  ${green}3.${plain} 卸载 v2node
————————————————
  ${green}4.${plain} 启动 v2node
  ${green}5.${plain} 停止 v2node
  ${green}6.${plain} 重启 v2node
  ${green}7.${plain} 查看 v2node 状态
  ${green}8.${plain} 查看 v2node 日志
————————————————
  ${green}9.${plain} 设置 v2node 开机自启
  ${green}10.${plain} 取消 v2node 开机自启
————————————————
  ${green}11.${plain} 查看 v2node 版本
  ${green}12.${plain} 升级 v2node 维护脚本
  ${green}13.${plain} 生成 v2node 配置文件
  ${green}14.${plain} 放行 VPS 的所有网络端口
————————————————
  ${green}15.${plain} 配置 Cloudflare DDNS
  ${green}16.${plain} 配置被墙检测/自动换IP
  ${green}17.${plain} 立即执行一次 DDNS/墙检测
  ${green}18.${plain} 只执行一次被墙检测/自动换IP
  ${green}19.${plain} 查看 DDNS/墙检测状态
  ${green}20.${plain} 停用 DDNS/墙检测
  ${green}21.${plain} 查看 Juicity observer 状态
  ${green}22.${plain} 查看外部协议状态
————————————————
  ${green}23.${plain} 退出脚本
 "
 #后续更新可加入上方字符串中
    show_status
    echo && read -rp "请输入选择 [0-23]: " num

    case "${num}" in
        0) config ;;
        1) check_uninstall && install ;;
        2) check_install && update ;;
        3) check_install && uninstall ;;
        4) check_install && start ;;
        5) check_install && stop ;;
        6) check_install && restart ;;
        7) check_install && status ;;
        8) check_install && show_log ;;
        9) check_install && enable ;;
        10) check_install && disable ;;
        11) check_install && show_v2node_version ;;
        12) update_shell ;;
        13) generate_config_file ;;
        14) open_ports ;;
        15) check_install && configure_cloudflare_ddns ;;
        16) check_install && configure_block_check ;;
        17) check_install && run_ddns_once ;;
        18) check_install && run_block_check_once ;;
        19) show_ddns_status ;;
        20) disable_ddns_monitor ;;
        21) show_observer_status ;;
        22) show_external_status ;;
        23) exit ;;
        *) echo -e "${red}请输入正确的数字 [0-23]${plain}" ;;
    esac
}


if [[ $# > 0 ]]; then
    case $1 in
        "start") check_install 0 && start 0 ;;
        "stop") check_install 0 && stop 0 ;;
        "restart") check_install 0 && restart 0 ;;
        "status") check_install 0 && status 0 ;;
        "enable") check_install 0 && enable 0 ;;
        "disable") check_install 0 && disable 0 ;;
        "log") check_install 0 && show_log 0 ;;
        "update") check_install 0 && update 0 $2 ;;
        "config") config $* ;;
        "generate") generate_config_file ;;
        "ddns") check_install 0 && configure_cloudflare_ddns $2 ;;
        "block-check") check_install 0 && configure_block_check $2 ;;
        "ddns-all") check_install 0 && configure_ddns_monitor $2 ;;
        "ddns-set") check_install 0 && configure_ddns_monitor_from_args "${@:2}" ;;
        "ddns-run") check_install 0 && run_ddns_once $2 ;;
        "block-check-run") check_install 0 && run_block_check_once $2 ;;
        "ddns-status") show_ddns_status $2 ;;
        "ddns-disable") disable_ddns_monitor $2 ;;
        "observer-status") show_observer_status $2 ;;
        "external-status") show_external_status $2 ;;
        "install") check_uninstall 0 && install 0 ;;
        "uninstall") check_install 0 && uninstall 0 ;;
        "version") check_install 0 && show_v2node_version 0 ;;
        "update_shell") update_shell ;;
        *) show_usage
    esac
else
    show_menu
fi
