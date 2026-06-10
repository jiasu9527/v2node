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

assert_contains "script/v2node.sh" 'v2node block-check-run'
assert_contains "script/v2node.sh" 'run_block_check_once'
assert_contains "script/v2node.sh" '只执行一次被墙检测/自动换IP'
assert_contains "script/v2node.sh" '"block-check-run") check_install 0 && run_block_check_once'

assert_contains "script/v2node-ddns.sh" 'v2node-ddns block-check-run'
assert_contains "script/v2node-ddns.sh" 'run_block_check_once'
assert_contains "script/v2node-ddns.sh" '仅执行被墙检测'

assert_contains "README.md" 'v2node block-check-run'
