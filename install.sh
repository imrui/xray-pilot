#!/usr/bin/env bash

set -euo pipefail

REPO="imrui/xray-pilot"
INSTALL_BIN="/usr/local/bin/xray-pilot"
CONFIG_DIR="/etc/xray-pilot"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
STATE_DIR="/var/lib/xray-pilot"
SERVICE_FILE="/etc/systemd/system/xray-pilot.service"
CREDS_FILE="${CONFIG_DIR}/install.env"
SERVICE_USER="xray-pilot"
SERVICE_GROUP="xray-pilot"

require_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    echo "Please run this installer as root, for example: curl ... | sudo bash" >&2
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

detect_arch() {
  local machine
  machine="$(uname -m)"
  case "${machine}" in
    x86_64|amd64)
      ARCH="amd64"
      ;;
    aarch64|arm64)
      ARCH="arm64"
      ;;
    *)
      echo "Unsupported architecture: ${machine}. Only amd64 and arm64 are supported." >&2
      exit 1
      ;;
  esac
}

detect_os() {
  local system
  system="$(uname -s)"
  case "${system}" in
    Linux)
      OS="linux"
      ;;
    *)
      echo "Unsupported operating system: ${system}. This installer only supports Linux." >&2
      exit 1
      ;;
  esac
}

get_latest_tag() {
  local api_url="https://api.github.com/repos/${REPO}/releases/latest"
  TAG="$(curl -fsSL "${api_url}" | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  if [[ -z "${TAG}" ]]; then
    echo "Unable to determine the latest release tag from ${api_url}" >&2
    exit 1
  fi
}

random_hex() {
  local bytes="$1"
  openssl rand -hex "${bytes}"
}

random_password() {
  openssl rand -base64 24 | tr -d '=+/' | cut -c1-24
}

download_release() {
  TMP_DIR="$(mktemp -d)"
  ARCHIVE="xray-pilot_${TAG}_${OS}_${ARCH}.tar.gz"
  ARCHIVE_URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"
  CHECKSUM_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"

  echo "Downloading ${ARCHIVE}..."
  curl -fsSL -o "${TMP_DIR}/${ARCHIVE}" "${ARCHIVE_URL}"
  curl -fsSL -o "${TMP_DIR}/checksums.txt" "${CHECKSUM_URL}"
}

verify_release() {
  (
    cd "${TMP_DIR}"
    sha256sum -c checksums.txt --ignore-missing
  )
}

install_binary() {
  tar -xzf "${TMP_DIR}/${ARCHIVE}" -C "${TMP_DIR}"
  install -m 0755 "${TMP_DIR}/xray-pilot" "${INSTALL_BIN}"
}

ensure_service_user() {
  if ! getent group "${SERVICE_GROUP}" >/dev/null 2>&1; then
    groupadd --system "${SERVICE_GROUP}"
  fi

  if ! id -u "${SERVICE_USER}" >/dev/null 2>&1; then
    useradd \
      --system \
      --gid "${SERVICE_GROUP}" \
      --home-dir "${STATE_DIR}" \
      --shell /usr/sbin/nologin \
      --comment "xray-pilot service user" \
      "${SERVICE_USER}"
  fi
}

write_config() {
  mkdir -p "${CONFIG_DIR}" "${STATE_DIR}"
  chown "${SERVICE_USER}:${SERVICE_GROUP}" "${STATE_DIR}"
  chown root:"${SERVICE_GROUP}" "${CONFIG_DIR}"
  chmod 0750 "${CONFIG_DIR}"

  if [[ ! -f "${CONFIG_FILE}" ]]; then
    JWT_SECRET="$(random_hex 32)"
    MASTER_KEY="$(random_hex 32)"
    ADMIN_PASSWORD="$(random_password)"

    cat >"${CONFIG_FILE}" <<EOF
server:
  port: 2026
  mode: release

database:
  driver: sqlite
  dsn: ${STATE_DIR}/xray-pilot.db

jwt:
  secret: "${JWT_SECRET}"
  expire: 24

crypto:
  master_key: "${MASTER_KEY}"

admins:
  - username: admin
    password: "${ADMIN_PASSWORD}"
EOF

    cat >"${CREDS_FILE}" <<EOF
XRAY_PILOT_ADMIN_USERNAME=admin
XRAY_PILOT_ADMIN_PASSWORD=${ADMIN_PASSWORD}
XRAY_PILOT_VERSION=${TAG}
EOF
  fi

  chown root:"${SERVICE_GROUP}" "${CONFIG_FILE}"
  chmod 0640 "${CONFIG_FILE}"
  if [[ -f "${CREDS_FILE}" ]]; then
    chown root:root "${CREDS_FILE}"
    chmod 0600 "${CREDS_FILE}"
  fi
}

write_service() {
  cat >"${SERVICE_FILE}" <<EOF
[Unit]
Description=xray-pilot service
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
WorkingDirectory=${CONFIG_DIR}
ExecStart=${INSTALL_BIN}
Restart=on-failure
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
}

enable_service() {
  systemctl daemon-reload
  systemctl enable --now xray-pilot.service
}

cleanup() {
  if [[ -n "${TMP_DIR:-}" && -d "${TMP_DIR}" ]]; then
    rm -rf "${TMP_DIR}"
  fi
}

main() {
  trap cleanup EXIT

  require_root
  require_cmd curl
  require_cmd tar
  require_cmd sha256sum
  require_cmd openssl
  require_cmd systemctl

  detect_os
  detect_arch
  get_latest_tag
  download_release
  verify_release
  install_binary
  ensure_service_user
  write_config
  write_service
  enable_service

  echo
  echo "xray-pilot ${TAG} installed successfully."
  echo "Binary: ${INSTALL_BIN}"
  echo "Config: ${CONFIG_FILE}"
  echo "Credentials: ${CREDS_FILE}"
  echo "Service: systemctl status xray-pilot.service"
}

main "$@"
