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

assert_not_contains() {
    local file="$1"
    local pattern="$2"
    if grep -Fq -- "$pattern" "${ROOT_DIR}/${file}"; then
        echo "unexpected pattern in ${file}: ${pattern}" >&2
        return 1
    fi
}

# New block-check installs should request a new IP immediately on a failed check.
assert_contains "script/install.sh" 'DDNS_BLOCK_CHECK_THRESHOLD_ARG="1"'
assert_contains "script/install.sh" 'DDNS_CHANGE_IP_WAIT_ARG="0"'
assert_contains "script/install.sh" 'DDNS_CHANGE_IP_COOLDOWN_ARG="0"'
assert_contains "script/install.sh" '--block-check-threshold 1'
assert_contains "script/install.sh" '--change-ip-wait 0'
assert_contains "script/install.sh" '--change-ip-cooldown 0'

# Interactive and non-interactive management defaults should match.
assert_contains "script/v2node.sh" 'local block_threshold=1'
assert_contains "script/v2node.sh" 'local change_wait=0'
assert_contains "script/v2node.sh" 'local change_cooldown=0'
assert_contains "script/v2node.sh" 'local block_threshold="1"'
assert_contains "script/v2node.sh" 'local change_wait="0"'
assert_contains "script/v2node.sh" 'local change_cooldown="0"'
assert_not_contains "script/v2node.sh" '默认3'
assert_not_contains "script/v2node.sh" '默认60'
assert_not_contains "script/v2node.sh" '默认1800'

# Runtime script fallbacks should not reintroduce a cooldown when values are absent/invalid.
assert_contains "script/v2node-ddns.sh" ': "${BLOCK_CHECK_FAIL_THRESHOLD:=1}"'
assert_contains "script/v2node-ddns.sh" ': "${CHANGE_IP_WAIT_SECONDS:=0}"'
assert_contains "script/v2node-ddns.sh" ': "${CHANGE_IP_COOLDOWN_SECONDS:=0}"'
assert_contains "script/v2node-ddns.sh" 'BLOCK_CHECK_FAIL_THRESHOLD=1'
assert_contains "script/v2node-ddns.sh" 'CHANGE_IP_WAIT_SECONDS=0'
assert_contains "script/v2node-ddns.sh" 'CHANGE_IP_COOLDOWN_SECONDS=0'
assert_contains "script/v2node-ddns.sh" 'threshold="${BLOCK_CHECK_FAIL_THRESHOLD:-1}"'
assert_contains "script/v2node-ddns.sh" 'cooldown="${CHANGE_IP_COOLDOWN_SECONDS:-0}"'

# Updating an existing install should migrate old block-check defaults as well.
assert_contains "script/install.sh" 'migrate_ddns_no_cooldown_config'
assert_contains "script/v2node.sh" 'migrate_ddns_no_cooldown_config'
