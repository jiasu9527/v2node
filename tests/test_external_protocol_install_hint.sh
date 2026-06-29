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

assert_contains "script/install.sh" 'show_external_protocol_hint()'
assert_contains "script/install.sh" 'Juicity/Mieru 外部协议提示'
assert_contains "script/install.sh" 'v2node external-status'
assert_contains "script/install.sh" 'juicity-server'
assert_contains "script/install.sh" 'mita'
assert_contains "script/install.sh" 'observer-status'
assert_contains "script/install.sh" 'show_external_protocol_hint'
