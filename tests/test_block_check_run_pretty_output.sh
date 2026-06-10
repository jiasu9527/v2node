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

assert_contains "script/v2node-ddns.sh" "pretty_line()"
assert_contains "script/v2node-ddns.sh" "pretty_title()"
assert_contains "script/v2node-ddns.sh" "pretty_kv()"
assert_contains "script/v2node-ddns.sh" "pretty_success()"
assert_contains "script/v2node-ddns.sh" "pretty_warn()"
assert_contains "script/v2node-ddns.sh" "V2NODE_FORCE_COLOR"
assert_contains "script/v2node-ddns.sh" "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
assert_contains "script/v2node-ddns.sh" "被墙检测 / 自动换 IP"
assert_contains "script/v2node-ddns.sh" "检测结果"
assert_contains "script/v2node-ddns.sh" "失败原因"
