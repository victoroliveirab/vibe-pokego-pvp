#!/usr/bin/env bash
set -euo pipefail

COMPOSE_CMD=${COMPOSE_CMD:-"docker compose"}
MAX_ATTEMPTS=${MAX_ATTEMPTS:-30}
SLEEP_SECONDS=${SLEEP_SECONDS:-2}
ENV_FILE=${ENV_FILE:-.env}

if [ -f "${ENV_FILE}" ]; then
  # shellcheck disable=SC1090
  source "${ENV_FILE}"
fi

WEB_PORT=${WEB_PORT:-8080}
FRONTEND_PORT=${FRONTEND_PORT:-4173}
WORKER_HEALTH_PORT=${WORKER_HEALTH_PORT:-8081}
DATABASE_PATH=${DATABASE_PATH:-./var/app.db}
DATABASE_URL=${DATABASE_URL:-}
UPLOAD_STORAGE_MODE=${UPLOAD_STORAGE_MODE:-local}
UPLOAD_LOCAL_DIR=${UPLOAD_LOCAL_DIR:-./testdata/uploads}
UPLOADTHING_TOKEN=${UPLOADTHING_TOKEN:-}
UPLOADTHING_REQUEST_TIMEOUT_SECS=${UPLOADTHING_REQUEST_TIMEOUT_SECS:-30}
WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS=${WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS:-30}
WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT=${WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT:-2}
WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR=${WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR:-}

is_remote_database_url() {
  case "${DATABASE_URL}" in
    libsql://*|http://*|https://*|ws://*|wss://*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

fail() {
  echo "ERROR: $1" >&2
  echo "Current compose status:" >&2
  ${COMPOSE_CMD} ps >&2 || true
  exit 1
}

is_positive_int() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    0) return 1 ;;
    *) return 0 ;;
  esac
}

is_non_negative_int() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    *) return 0 ;;
  esac
}

for svc in web worker frontend; do
  if ! ${COMPOSE_CMD} ps --services --status running | grep -qx "${svc}"; then
    fail "${svc} service is not running"
  fi
done

echo "Waiting for service health checks..."
attempt=0
while [ "${attempt}" -lt "${MAX_ATTEMPTS}" ]; do
  status="$(${COMPOSE_CMD} ps --format '{{.Service}} {{.State}} {{.Health}}')"
  echo "${status}"

  web_health="$(echo "${status}" | awk '$1=="web" {print $3}')"
  worker_health="$(echo "${status}" | awk '$1=="worker" {print $3}')"
  frontend_health="$(echo "${status}" | awk '$1=="frontend" {print $3}')"

  if [ "${web_health}" = "healthy" ] && [ "${worker_health}" = "healthy" ] && [ "${frontend_health}" = "healthy" ]; then
    break
  fi

  attempt=$((attempt + 1))
  sleep "${SLEEP_SECONDS}"
done

if [ "${attempt}" -eq "${MAX_ATTEMPTS}" ]; then
  fail "timed out waiting for all service health checks to become healthy"
fi

echo "Verifying health endpoints..."
curl -fsS "http://127.0.0.1:${WEB_PORT}/healthz" >/dev/null || fail "web health endpoint check failed"
curl -fsS "http://127.0.0.1:${WORKER_HEALTH_PORT}/healthz" >/dev/null || fail "worker health endpoint check failed"
curl -fsS "http://127.0.0.1:${FRONTEND_PORT}/" >/dev/null || fail "frontend endpoint check failed"

echo "Verifying persistence paths..."
if is_remote_database_url; then
  echo "DATABASE_URL is remote (${DATABASE_URL}); skipping local database file checks."
elif [ ! -f "${DATABASE_PATH}" ]; then
  fail "database file is missing at ${DATABASE_PATH}"
fi

case "${UPLOAD_STORAGE_MODE}" in
  local)
    if [ -z "${UPLOAD_LOCAL_DIR}" ]; then
      fail "UPLOAD_LOCAL_DIR must be set when UPLOAD_STORAGE_MODE=local"
    fi

    if [ ! -d "${UPLOAD_LOCAL_DIR}" ]; then
      fail "local upload directory is missing at ${UPLOAD_LOCAL_DIR} (UPLOAD_STORAGE_MODE=local)"
    fi

    probe_file="${UPLOAD_LOCAL_DIR}/.smoke-writecheck-$$"
    if ! touch "${probe_file}" >/dev/null 2>&1; then
      fail "local upload directory is not writable at ${UPLOAD_LOCAL_DIR} (UPLOAD_STORAGE_MODE=local)"
    fi
    rm -f "${probe_file}"
    ;;
  uploadthing)
    if [ -z "${UPLOADTHING_TOKEN}" ]; then
      fail "UPLOADTHING_TOKEN must be set when UPLOAD_STORAGE_MODE=uploadthing"
    fi
    if ! is_positive_int "${UPLOADTHING_REQUEST_TIMEOUT_SECS}"; then
      fail "UPLOADTHING_REQUEST_TIMEOUT_SECS must be a positive integer"
    fi
    if ! is_positive_int "${WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS}"; then
      fail "WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS must be a positive integer"
    fi
    if ! is_non_negative_int "${WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT}"; then
      fail "WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT must be zero or greater"
    fi
    if [ -n "${WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR}" ]; then
      mkdir -p "${WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR}" 2>/dev/null || fail "WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR is not creatable"
      probe_file="${WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR}/.smoke-writecheck-$$"
      if ! touch "${probe_file}" >/dev/null 2>&1; then
        fail "WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR is not writable at ${WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR}"
      fi
      rm -f "${probe_file}"
    fi
    echo "UPLOAD_STORAGE_MODE=uploadthing; validated uploadthing credentials and worker download settings."
    ;;
  *)
    fail "UPLOAD_STORAGE_MODE must be one of: local, uploadthing (got: ${UPLOAD_STORAGE_MODE})"
    ;;
esac

if [ "${UPLOAD_STORAGE_MODE}" = "local" ]; then
  echo "Local upload directory is valid at ${UPLOAD_LOCAL_DIR}."
fi

echo "Smoke verification passed."
