#!/usr/bin/env bash
set -euo pipefail

assert_contains() {
    local file="$1"
    local needle="$2"
    if ! grep -Fq "$needle" "$file"; then
        echo "missing expected text: $needle" >&2
        exit 1
    fi
}

assert_contains "script/v2node.sh" 'v2node external-status - 查看 Juicity/Mieru 外部协议状态'
assert_contains "script/v2node.sh" 'show_external_status()'
assert_contains "script/v2node.sh" 'external-juicity-*.json'
assert_contains "script/v2node.sh" 'external-mieru-*.json'
assert_contains "script/v2node.sh" 'command -v juicity-server'
assert_contains "script/v2node.sh" 'command -v mita'
assert_contains "script/v2node.sh" 'pgrep -af "juicity-server|mita"'
assert_contains "script/v2node.sh" 'mita get metrics'
assert_contains "script/v2node.sh" 'ss -lntup'
assert_contains "script/v2node.sh" '"external-status") show_external_status'
assert_contains "script/v2node.sh" '查看外部协议状态'
