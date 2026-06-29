#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
install="$root/script/install.sh"
manager="$root/script/v2node.sh"

assert_contains() {
  local file="$1"
  local pattern="$2"
  if ! grep -q -- "$pattern" "$file"; then
    echo "missing '$pattern' in $file" >&2
    exit 1
  fi
}

assert_contains "$install" "--enable-juicity"
assert_contains "$install" "--enable-mieru"
assert_contains "$install" "install_juicity"
assert_contains "$install" "install_mieru"
assert_contains "$manager" "--enable-juicity"
assert_contains "$manager" "--enable-mieru"

echo "external protocol script assertions passed"
