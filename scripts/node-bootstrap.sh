#!/usr/bin/env bash

set -euo pipefail

XRAY_INSTALL_URL="${XRAY_INSTALL_URL:-https://github.com/XTLS/Xray-install/raw/main/install-release.sh}"
AUTHORIZED_KEYS_INPUT="${AUTHORIZED_KEYS:-}"
ENABLE_BBR="${ENABLE_BBR:-true}"
ALLOW_ROOT_LOGIN="${ALLOW_ROOT_LOGIN:-yes}"
ROOT_HOME="${ROOT_HOME:-/root}"
SSH_CONFIG_FILE="${SSH_CONFIG_FILE:-/etc/ssh/sshd_config}"
AUTHORIZED_KEYS_FILE="${AUTHORIZED_KEYS_FILE:-${ROOT_HOME}/.ssh/authorized_keys}"
SSH_CONFIG_BACKUP=""

log_step() {
  echo
  echo "==> $1"
}

log_done() {
  echo "--> $1"
}

backup_file() {
  local source="$1"
  local timestamp

  timestamp="$(date +%Y%m%d-%H%M%S)"
  SSH_CONFIG_BACKUP="${source}.bak.${timestamp}"
  cp "${source}" "${SSH_CONFIG_BACKUP}"
}

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    echo "Please run this script as root." >&2
    exit 1
  fi
}

require_cmd() {
  local cmd="$1"
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "Missing required command: ${cmd}" >&2
    exit 1
  fi
}

collect_authorized_keys() {
  if [[ -n "${AUTHORIZED_KEYS_INPUT}" ]]; then
    return
  fi

  echo "Paste management public keys for root authorized_keys, one per line."
  echo "Press Enter on an empty line to finish. Leave it empty to skip this step."

  local line
  local collected=()
  while IFS= read -r line; do
    if [[ -z "${line}" ]]; then
      break
    fi
    collected+=("${line}")
  done

  if [[ "${#collected[@]}" -gt 0 ]]; then
    AUTHORIZED_KEYS_INPUT="$(printf '%s\n' "${collected[@]}")"
  fi
}

upsert_sshd_option() {
  local key="$1"
  local value="$2"
  local tmp_file
  local updated=0

  tmp_file="$(mktemp)"

  awk -v key="${key}" -v value="${value}" '
    BEGIN {
      updated = 0
      in_match = 0
    }
    /^[[:space:]]*Match([[:space:]]|$)/ {
      in_match = 1
    }
    {
      if (!in_match && $0 ~ ("^[#[:space:]]*" key "[[:space:]]+")) {
        if (updated == 0) {
          print key " " value
          updated = 1
        } else {
          print "#" $0
        }
        next
      }
      print
    }
    END {
      if (updated == 0) {
        print ""
        print key " " value
      }
    }
  ' "${SSH_CONFIG_FILE}" >"${tmp_file}"

  mv "${tmp_file}" "${SSH_CONFIG_FILE}"
}

restart_ssh_service() {
  if systemctl restart ssh >/dev/null 2>&1; then
    echo "ssh"
    return
  fi
  if systemctl restart sshd >/dev/null 2>&1; then
    echo "sshd"
    return
  fi
  if service ssh restart >/dev/null 2>&1; then
    echo "ssh"
    return
  fi
  if service sshd restart >/dev/null 2>&1; then
    echo "sshd"
    return
  fi
  return 1
}

configure_root_ssh() {
  if [[ -z "${AUTHORIZED_KEYS_INPUT}" ]]; then
    echo "No management public keys provided. Please verify ${AUTHORIZED_KEYS_FILE} manually before using node sync."
    return
  fi

  mkdir -p "${ROOT_HOME}/.ssh"
  chmod 700 "${ROOT_HOME}/.ssh"
  touch "${AUTHORIZED_KEYS_FILE}"

  while IFS= read -r pubkey; do
    if [[ -z "${pubkey}" ]]; then
      continue
    fi
    if ! grep -Fqx "${pubkey}" "${AUTHORIZED_KEYS_FILE}"; then
      printf '%s\n' "${pubkey}" >>"${AUTHORIZED_KEYS_FILE}"
    fi
  done <<<"${AUTHORIZED_KEYS_INPUT}"

  chmod 600 "${AUTHORIZED_KEYS_FILE}"
}

install_xray() {
  bash -c "$(curl -L "${XRAY_INSTALL_URL}")" @ install
  systemctl enable xray >/dev/null 2>&1 || true
  systemctl restart xray
}

enable_bbr() {
  if [[ "${ENABLE_BBR}" != "true" ]]; then
    return
  fi

  if ! grep -Fqx 'net.core.default_qdisc=fq' /etc/sysctl.conf; then
    echo 'net.core.default_qdisc=fq' >>/etc/sysctl.conf
  fi
  if ! grep -Fqx 'net.ipv4.tcp_congestion_control=bbr' /etc/sysctl.conf; then
    echo 'net.ipv4.tcp_congestion_control=bbr' >>/etc/sysctl.conf
  fi
  sysctl -p
}

main() {
  local ssh_service_name=""

  log_step "Checking prerequisites"
  require_root
  require_cmd curl
  require_cmd sed
  require_cmd systemctl
  require_cmd grep
  require_cmd awk
  log_done "Prerequisites ready"

  log_step "Collecting management public keys"
  collect_authorized_keys
  if [[ -n "${AUTHORIZED_KEYS_INPUT}" ]]; then
    log_done "Management public keys collected"
  else
    log_done "Skipped management public keys input"
  fi

  log_step "Updating sshd_config"
  backup_file "${SSH_CONFIG_FILE}"
  log_done "Backup created at ${SSH_CONFIG_BACKUP}"
  upsert_sshd_option "PermitRootLogin" "${ALLOW_ROOT_LOGIN}"
  log_done "PermitRootLogin set to ${ALLOW_ROOT_LOGIN}"

  log_step "Configuring root authorized_keys"
  configure_root_ssh
  log_done "authorized_keys step completed"

  log_step "Installing Xray"
  install_xray
  log_done "Xray installed and restarted"

  log_step "Applying BBR settings"
  enable_bbr
  if [[ "${ENABLE_BBR}" == "true" ]]; then
    log_done "BBR settings applied"
  else
    log_done "Skipped BBR settings"
  fi

  log_step "Restarting SSH service"
  if ssh_service_name="$(restart_ssh_service)"; then
    log_done "SSH service restarted via ${ssh_service_name}"
  else
    echo "WARNING: Unable to restart ssh.service or sshd.service automatically." >&2
    echo "Please verify the SSH service name manually and restart it if needed." >&2
  fi

  echo
  echo "Node bootstrap completed."
  echo "sshd_config backup: ${SSH_CONFIG_BACKUP}"
  echo "PermitRootLogin: ${ALLOW_ROOT_LOGIN}"
  echo "Authorized keys file: ${AUTHORIZED_KEYS_FILE}"
  echo "Xray service: $(systemctl is-active xray 2>/dev/null || echo unknown)"
  echo "Xray version: $(xray version 2>/dev/null | head -n 1 || echo unknown)"
  if [[ "${ENABLE_BBR}" == "true" ]]; then
    echo "TCP congestion control: $(sysctl -n net.ipv4.tcp_congestion_control 2>/dev/null || echo unknown)"
  fi
}

main "$@"
