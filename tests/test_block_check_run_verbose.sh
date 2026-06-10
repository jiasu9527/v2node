#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

assert_contains() {
    local file="$1"
    local pattern="$2"
    if ! grep -Fq -- "$pattern" "${ROOT_DIR}/${file}"; then
        echo "missing pattern in ${file}: ${pattern}" >&2
        return 1
    fi
}

assert_contains "script/v2node-ddns.sh" "被墙检测 / 自动换 IP"
assert_contains "script/v2node-ddns.sh" "当前公网 IP"
assert_contains "script/v2node-ddns.sh" "检测接口"
assert_contains "script/v2node-ddns.sh" "检测结果: 正常"
assert_contains "script/v2node-ddns.sh" "检测结果: 异常/疑似被墙"
assert_contains "script/v2node-ddns.sh" "未达到换 IP 阈值"
assert_contains "script/v2node-ddns.sh" "换 IP 前公网 IP"
assert_contains "script/v2node-ddns.sh" "换 IP 后公网 IP"
