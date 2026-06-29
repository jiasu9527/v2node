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

assert_contains "script/v2node.sh" 'v2node observer-status - 查看 Juicity observer 采集状态'
assert_contains "script/v2node.sh" 'show_observer_status()'
assert_contains "script/v2node.sh" 'external-juicity-*.observe.jsonl'
assert_contains "script/v2node.sh" 'v2node_observer_log'
assert_contains "script/v2node.sh" 'observer_log'
assert_contains "script/v2node.sh" 'tail -n 20 "$log_file"'
assert_contains "script/v2node.sh" '"observer-status") show_observer_status'
assert_contains "script/v2node.sh" '查看 Juicity observer 状态'
