#!/usr/bin/env bash

# Run with: bash scripts/node-bootstrap_test.sh
# All network calls, installation and service operations below are mocked.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
unset XRAY_VERSION
# shellcheck source=node-bootstrap.sh
source "${SCRIPT_DIR}/node-bootstrap.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

curl() {
  [[ "$*" == "-fsSL ${XRAY_INSTALL_URL}" ]] || return 2
  case "${MOCK_DOWNLOAD:-ok}" in
    fail) return 22 ;;
    empty) return 0 ;;
  esac
  printf '%s\n' 'printf "INSTALL_ARGS: %s %s %s\n" "$@"; exit "${MOCK_INSTALL_EXIT:-0}"'
}

xray() {
  [[ "${MOCK_VERSION_FAILURE:-false}" == false ]] || return 1
  printf 'Xray %s (mock)\n' "${MOCK_INSTALLED_VERSION:-${XRAY_VERSION}}"
}

systemctl() {
  echo "SYSTEMCTL: $*"
}

assert_install() {
  local expected="$1" output
  validate_xray_version
  [[ "${XRAY_VERSION}" == "${expected}" ]] || fail "version normalization"
  output="$(install_xray 2>&1)" || fail "installation of ${expected}: ${output}"
  [[ "${output}" == *"INSTALL_ARGS: install --version v${expected}"* ]] || fail "version not passed to installer"
  [[ "${output}" == *"SYSTEMCTL: restart xray"* ]] || fail "Xray was not restarted"
}

assert_install_failure() {
  local expected="$1" output
  if output="$(install_xray 2>&1)"; then
    fail "installation unexpectedly succeeded: ${expected}"
  fi
  [[ "${output}" == *"${expected}"* ]] || fail "missing failure message: ${output}"
  [[ "${output}" != *"SYSTEMCTL:"* ]] || fail "service operation after failure"
}

assert_install 26.3.27
XRAY_VERSION=26.4.17
assert_install 26.4.17
XRAY_VERSION=v26.3.27
assert_install 26.3.27

for invalid in latest 26.3 26.3.27-beta '26.3.27; echo unsafe' '26.3.27 26.4.17' ''; do
  XRAY_VERSION="${invalid}"
  if validate_xray_version >/dev/null 2>&1; then
    fail "invalid version accepted: ${invalid}"
  fi
done

# A malformed version must fail before even checking root, and the entry point
# must run both as a file and through stdin (the panel's curl | bash command).
for mode in file stdin; do
  set +e
  if [[ "${mode}" == file ]]; then
    output="$(XRAY_VERSION=latest bash "${SCRIPT_DIR}/node-bootstrap.sh" 2>&1)"
  else
    output="$(XRAY_VERSION=latest bash <"${SCRIPT_DIR}/node-bootstrap.sh" 2>&1)"
  fi
  status=$?
  set -e
  [[ "${status}" -ne 0 && "${output}" == *"Invalid XRAY_VERSION"* ]] || fail "entry point validation (${mode})"
done

XRAY_VERSION=26.3.27
MOCK_DOWNLOAD=fail
assert_install_failure 'Failed to download'
MOCK_DOWNLOAD=empty
assert_install_failure 'response is empty'
MOCK_DOWNLOAD=ok
export MOCK_INSTALL_EXIT=1
assert_install_failure 'Failed to install Xray'
export MOCK_INSTALL_EXIT=0
MOCK_INSTALLED_VERSION=26.4.17
assert_install_failure 'Xray version mismatch'
unset MOCK_INSTALLED_VERSION
MOCK_VERSION_FAILURE=true
assert_install_failure 'Failed to read the installed Xray version'

echo 'PASS: stable/default/custom versions, validation, stdin entry point and installation failures'
