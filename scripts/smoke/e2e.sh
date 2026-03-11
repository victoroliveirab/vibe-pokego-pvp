#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

if [ -f .env ]; then
  # shellcheck disable=SC1091
  source .env
fi

WEB_PORT=${WEB_PORT:-8080}
WEB_BASE_URL=${WEB_BASE_URL:-"http://127.0.0.1:${WEB_PORT}"}
POLL_ATTEMPTS=${POLL_ATTEMPTS:-300}
POLL_SLEEP_SECS=${POLL_SLEEP_SECS:-1}
DATABASE_PATH=${DATABASE_PATH:-./var/app.db}
DATABASE_URL=${DATABASE_URL:-}
QUEUE_IDLE_ATTEMPTS=${QUEUE_IDLE_ATTEMPTS:-180}
QUEUE_IDLE_SLEEP_SECS=${QUEUE_IDLE_SLEEP_SECS:-2}
PVP_EVAL_ATTEMPTS=${PVP_EVAL_ATTEMPTS:-120}
PVP_EVAL_SLEEP_SECS=${PVP_EVAL_SLEEP_SECS:-1}
SMOKE_RESET_NONTERMINAL_JOBS=${SMOKE_RESET_NONTERMINAL_JOBS:-1}
MANIFEST_PATH="testdata/fixtures/e2e-fixture-manifest.json"

require_cmds=(curl jq mktemp awk base64)
for cmd in "${require_cmds[@]}"; do
  if ! command -v "${cmd}" >/dev/null 2>&1; then
    echo "ERROR: required command not found: ${cmd}" >&2
    exit 1
  fi
done

fail() {
  echo "ERROR: $1" >&2
  exit 1
}

sha256_file() {
  local file_path="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "${file_path}" | awk '{print $1}'
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${file_path}" | awk '{print $1}'
    return
  fi
  fail "shasum or sha256sum is required for fixture verification"
}

decode_base64() {
  local value="$1"
  if echo "${value}" | base64 --decode >/dev/null 2>&1; then
    echo "${value}" | base64 --decode
    return
  fi
  echo "${value}" | base64 -D
}

verify_e2e_fixtures() {
  if [ ! -f "${MANIFEST_PATH}" ]; then
    fail "fixture manifest missing: ${MANIFEST_PATH}"
  fi

  local encoded item scenario_id path expected_sha actual_sha
  echo "Verifying e2e fixture assets..."
  while IFS= read -r encoded; do
    item="$(decode_base64 "${encoded}")"
    scenario_id="$(echo "${item}" | jq -r '.scenarioId')"
    path="$(echo "${item}" | jq -r '.path')"
    expected_sha="$(echo "${item}" | jq -r '.sha256')"

    if [ ! -f "${path}" ]; then
      fail "${scenario_id} asset missing at ${path}"
    fi

    actual_sha="$(sha256_file "${path}")"
    if [ "${actual_sha}" != "${expected_sha}" ]; then
      echo "ERROR: ${scenario_id} checksum mismatch" >&2
      echo "  path: ${path}" >&2
      echo "  expected: ${expected_sha}" >&2
      echo "  actual:   ${actual_sha}" >&2
      exit 1
    fi
    echo "  OK ${scenario_id} (${path})"
  done < <(jq -r '.assets[] | @base64' "${MANIFEST_PATH}")

  local fixture_path fixture_type
  echo "Verifying e2e expected fixture files..."
  while IFS= read -r fixture_path; do
    if [ ! -f "${fixture_path}" ]; then
      fail "expected fixture missing at ${fixture_path}"
    fi

    jq -e '.' "${fixture_path}" >/dev/null
    scenario_id="$(jq -r '.scenarioId // ""' "${fixture_path}")"
    fixture_type="$(jq -r '.type // ""' "${fixture_path}")"

    if [ -z "${scenario_id}" ] || [ -z "${fixture_type}" ]; then
      fail "${fixture_path} must define non-empty scenarioId and type"
    fi

    echo "  OK ${fixture_path} (${scenario_id}, ${fixture_type})"
  done < <(jq -r '.expectedFixtures[]' "${MANIFEST_PATH}")

  echo "e2e fixture verification passed."
}

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

if is_remote_database_url; then
  fail "e2e smoke suite requires local file-backed DATABASE_PATH. Remote DATABASE_URL detected: ${DATABASE_URL}"
fi

wait_for_idle_queue() {
  if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "sqlite3 not found; skipping queue-idle preflight check."
    return
  fi
  if [ ! -f "${DATABASE_PATH}" ]; then
    echo "database not found at ${DATABASE_PATH}; skipping queue-idle preflight check."
    return
  fi

  local attempt
  for attempt in $(seq 1 "${QUEUE_IDLE_ATTEMPTS}"); do
    local queued_count
    queued_count="$(sqlite3 -cmd '.timeout 5000' "${DATABASE_PATH}" \
      "SELECT COUNT(*) FROM jobs WHERE status IN ('QUEUED','PROCESSING');" 2>/dev/null | awk 'NR==1 {print $1}')"
    queued_count="${queued_count:-0}"

    if [ "${queued_count}" = "0" ]; then
      echo "Queue is idle."
      return
    fi

    echo "Waiting for queue to drain before e2e smoke run... queued_or_processing=${queued_count} (attempt ${attempt}/${QUEUE_IDLE_ATTEMPTS})"
    sleep "${QUEUE_IDLE_SLEEP_SECS}"
  done

  fail "queue did not drain before smoke run (DATABASE_PATH=${DATABASE_PATH})"
}

reset_nonterminal_jobs_if_enabled() {
  if [ "${SMOKE_RESET_NONTERMINAL_JOBS}" != "1" ]; then
    return
  fi
  if ! command -v sqlite3 >/dev/null 2>&1; then
    return
  fi
  if [ ! -f "${DATABASE_PATH}" ]; then
    return
  fi

  local now_utc
  now_utc="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  local reset_count
  reset_count="$(sqlite3 -cmd '.timeout 5000' "${DATABASE_PATH}" "
UPDATE jobs
SET status = 'FAILED',
    error_code = 'SMOKE_QUEUE_RESET',
    error_message = 'Reset by e2e smoke preflight',
    finished_at = COALESCE(finished_at, '${now_utc}'),
    updated_at = '${now_utc}'
WHERE status IN ('QUEUED', 'PROCESSING');
SELECT changes();
")"

  reset_count="${reset_count:-0}"
  if [ "${reset_count}" != "0" ]; then
    echo "Reset ${reset_count} nonterminal job(s) before e2e smoke run."
  fi
}

assert_uuid_v4() {
  local value="$1"
  if [[ ! "${value}" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]]; then
    fail "expected UUIDv4 session id, got: ${value}"
  fi
}

create_session() {
  local body_file
  body_file="$(mktemp)"

  local status
  status="$(curl -sS -o "${body_file}" -w "%{http_code}" -X POST "${WEB_BASE_URL}/session")"
  if [ "${status}" != "201" ]; then
    cat "${body_file}" >&2
    rm -f "${body_file}"
    fail "POST /session returned status ${status}, expected 201"
  fi

  local session_id
  session_id="$(jq -r '.sessionId // empty' "${body_file}")"
  rm -f "${body_file}"

  if [ -z "${session_id}" ]; then
    fail "POST /session response missing sessionId"
  fi

  assert_uuid_v4 "${session_id}"
  echo "${session_id}"
}

upload_media() {
  local session_id="$1"
  local media_path="$2"
  local content_type="$3"

  local body_file
  body_file="$(mktemp)"

  local status
  status="$(curl -sS -o "${body_file}" -w "%{http_code}" -X POST \
    -H "X-Session-Id: ${session_id}" \
    -F "file=@${media_path};type=${content_type}" \
    "${WEB_BASE_URL}/uploads")"

  if [ "${status}" != "201" ]; then
    cat "${body_file}" >&2
    rm -f "${body_file}"
    fail "POST /uploads returned status ${status}, expected 201"
  fi

  local upload_id
  local job_id
  upload_id="$(jq -r '.uploadId // empty' "${body_file}")"
  job_id="$(jq -r '.jobId // empty' "${body_file}")"
  rm -f "${body_file}"

  if [ -z "${upload_id}" ] || [ -z "${job_id}" ]; then
    fail "POST /uploads missing uploadId/jobId"
  fi

  echo "${upload_id}|${job_id}"
}

poll_job_terminal() {
  local session_id="$1"
  local job_id="$2"
  local scenario_id="$3"

  local response=""
  local status=""
  local attempt

  for attempt in $(seq 1 "${POLL_ATTEMPTS}"); do
    response="$(curl -fsS -H "X-Session-Id: ${session_id}" "${WEB_BASE_URL}/jobs/${job_id}")"
    status="$(echo "${response}" | jq -r '.status // empty')"

    echo "[${scenario_id}] poll=${attempt} status=${status}" >&2

    case "${status}" in
      SUCCEEDED|FAILED|PENDING_USER_DEDUP)
        echo "${response}"
        return 0
        ;;
    esac

    sleep "${POLL_SLEEP_SECS}"
  done

  fail "timeout waiting for terminal status for scenario ${scenario_id}, job ${job_id}"
}

assert_job_expectation() {
  local expected_file="$1"
  local terminal_job_json="$2"

  local expected_status
  expected_status="$(jq -r '.jobExpectation.terminalStatus' "${expected_file}")"
  local actual_status
  actual_status="$(echo "${terminal_job_json}" | jq -r '.status // empty')"

  if [ "${actual_status}" != "${expected_status}" ]; then
    fail "scenario $(jq -r '.scenarioId' "${expected_file}"): expected terminal status ${expected_status}, got ${actual_status}"
  fi

  local expected_error_code
  expected_error_code="$(jq -r '.jobExpectation.errorCode // ""' "${expected_file}")"
  local actual_error_code
  actual_error_code="$(echo "${terminal_job_json}" | jq -r '.error.code // ""')"

  if [ -n "${expected_error_code}" ] && [ "${actual_error_code}" != "${expected_error_code}" ]; then
    fail "scenario $(jq -r '.scenarioId' "${expected_file}"): expected error code ${expected_error_code}, got ${actual_error_code}"
  fi

  if [ -z "${expected_error_code}" ] && [ -n "${actual_error_code}" ]; then
    fail "scenario $(jq -r '.scenarioId' "${expected_file}"): expected no error code, got ${actual_error_code}"
  fi

  local expected_error_message_contains
  expected_error_message_contains="$(jq -r '.jobExpectation.errorMessageContains // ""' "${expected_file}")"
  if [ -n "${expected_error_message_contains}" ]; then
    local actual_error_message
    actual_error_message="$(echo "${terminal_job_json}" | jq -r '.error.message // ""')"
    if [[ "${actual_error_message}" != *"${expected_error_message_contains}"* ]]; then
      fail "scenario $(jq -r '.scenarioId' "${expected_file}"): expected error message containing '${expected_error_message_contains}', got '${actual_error_message}'"
    fi
  fi

  if [ "${expected_status}" = "PENDING_USER_DEDUP" ]; then
    local progress
    progress="$(echo "${terminal_job_json}" | jq -r '.progress // -1')"
    if [ "${progress}" != "100" ]; then
      fail "pending-user-dedup terminal invariant failed: expected progress=100, got ${progress}"
    fi

    if ! echo "${terminal_job_json}" | jq -e '.stage == null' >/dev/null; then
      fail "pending-user-dedup terminal invariant failed: expected stage=null"
    fi

    if ! echo "${terminal_job_json}" | jq -e '.finishedAt != null' >/dev/null; then
      fail "pending-user-dedup terminal invariant failed: expected finishedAt to be set"
    fi

    if ! echo "${terminal_job_json}" | jq -e '.error == null' >/dev/null; then
      fail "pending-user-dedup terminal invariant failed: expected error=null"
    fi
  fi
}

fetch_pokemon() {
  local session_id="$1"
  curl -fsS -H "X-Session-Id: ${session_id}" "${WEB_BASE_URL}/pokemon"
}

check_pokemon_expectation() {
  local expected_file="$1"
  local pokemon_json="$2"
  local scenario_id
  scenario_id="$(jq -r '.scenarioId' "${expected_file}")"

  local min_results
  min_results="$(jq -r '.pokemonExpectation.minResults // 0' "${expected_file}")"
  local actual_results
  actual_results="$(echo "${pokemon_json}" | jq '.results | length')"

  if [ "${actual_results}" -lt "${min_results}" ]; then
    echo "scenario ${scenario_id}: expected at least ${min_results} pokemon results, got ${actual_results}"
    return 1
  fi

  while IFS= read -r species; do
    if [ -z "${species}" ]; then
      continue
    fi

    if echo "${pokemon_json}" | jq -e --arg species "${species}" 'any(.results[]?; .speciesName == $species)' >/dev/null; then
      echo "scenario ${scenario_id}: did not expect species '${species}' in /pokemon results"
      return 1
    fi
  done < <(jq -r '.pokemonExpectation.mustNotContainSpecies[]? // empty' "${expected_file}")

  while IFS= read -r expected_entry; do
    if [ -z "${expected_entry}" ]; then
      continue
    fi

    if ! echo "${pokemon_json}" | jq -e --argjson expected "${expected_entry}" '
      def eval_match($want; $actual):
        (($want.maxCp? // null) == null or $actual.maxCp == $want.maxCp)
        and (($want.evaluatedSpeciesId? // null) == null or $actual.evaluatedSpeciesId == $want.evaluatedSpeciesId)
        and (($want.rank? // null) == null or $actual.rank == $want.rank)
        and (($want.rankMin? // null) == null or $actual.rank >= $want.rankMin)
        and (($want.rankMax? // null) == null or $actual.rank <= $want.rankMax)
        and (($want.percentageMin? // null) == null or $actual.percentage >= $want.percentageMin)
        and (($want.percentageMax? // null) == null or $actual.percentage <= $want.percentageMax)
        and (($want.bestCp? // null) == null or $actual.bestCp == $want.bestCp)
        and (($want.bestLevel? // null) == null or $actual.bestLevel == $want.bestLevel)
        and (($want.bestLevelMin? // null) == null or $actual.bestLevel >= $want.bestLevelMin)
        and (($want.bestLevelMax? // null) == null or $actual.bestLevel <= $want.bestLevelMax)
        and (($want.statProduct? // null) == null or $actual.statProduct == $want.statProduct)
        and (($want.statProductMin? // null) == null or $actual.statProduct >= $want.statProductMin)
        and (($want.statProductMax? // null) == null or $actual.statProduct <= $want.statProductMax);

      any(.results[]?;
        . as $result
        | $result.speciesName == $expected.speciesName
        and $result.cp == $expected.cp
        and $result.hp == $expected.hp
        and $result.ivs.attack == $expected.ivs.attack
        and $result.ivs.defense == $expected.ivs.defense
        and $result.ivs.stamina == $expected.ivs.stamina
        and (($expected.maxCpEvaluationsMinCount // 0) <= (($result.maxCpEvaluations // []) | length))
        and all(($expected.maxCpEvaluationsMustContain // [])[]?;
          . as $want
          | any(($result.maxCpEvaluations // [])[]?; eval_match($want; .))
        )
      )
    ' >/dev/null; then
      echo "scenario ${scenario_id}: expected pokemon entry not found in /pokemon"
      echo "entry: ${expected_entry}"
      return 1
    fi
  done < <(jq -c '.pokemonExpectation.mustContain[]? // empty' "${expected_file}")

  return 0
}

assert_pokemon_expectation() {
  local expected_file="$1"
  local pokemon_json="$2"

  local reason
  if ! reason="$(check_pokemon_expectation "${expected_file}" "${pokemon_json}")"; then
    fail "${reason}"
  fi
}

has_pvp_expectations() {
  local expected_file="$1"
  jq -e '
    any(.pokemonExpectation.mustContain[]?; ((.maxCpEvaluationsMinCount // 0) > 0) or (((.maxCpEvaluationsMustContain // []) | length) > 0))
  ' "${expected_file}" >/dev/null
}

wait_for_session_pvp_queue_completion() {
  local session_id="$1"
  local expected_file="$2"
  local scenario_id="$3"

  if ! has_pvp_expectations "${expected_file}"; then
    return
  fi

  if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "sqlite3 not found; skipping pvp queue completion wait for scenario ${scenario_id}."
    return
  fi
  if [ ! -f "${DATABASE_PATH}" ]; then
    echo "database not found at ${DATABASE_PATH}; skipping pvp queue completion wait for scenario ${scenario_id}."
    return
  fi

  local attempt total_results remaining
  for attempt in $(seq 1 "${PVP_EVAL_ATTEMPTS}"); do
    total_results="$(sqlite3 -cmd '.timeout 5000' "${DATABASE_PATH}" \
      "SELECT COUNT(*) FROM appraisal_results WHERE session_id = '${session_id}';" 2>/dev/null | awk 'NR==1 {print $1}')"
    total_results="${total_results:-0}"

    if [ "${total_results}" = "0" ]; then
      return
    fi

    remaining="$(sqlite3 -cmd '.timeout 5000' "${DATABASE_PATH}" \
      "SELECT COUNT(*) FROM appraisal_result_pvp_eval_queue q JOIN appraisal_results r ON r.id = q.appraisal_result_id WHERE r.session_id = '${session_id}' AND q.status != 'SUCCEEDED';" 2>/dev/null | awk 'NR==1 {print $1}')"
    remaining="${remaining:-0}"

    if [ "${remaining}" = "0" ]; then
      return
    fi

    echo "Waiting for session PvP queue completion... scenario=${scenario_id} remaining=${remaining} (attempt ${attempt}/${PVP_EVAL_ATTEMPTS})"
    sleep "${PVP_EVAL_SLEEP_SECS}"
  done

  fail "scenario ${scenario_id}: timed out waiting for session PvP queue completion"
}

run_session_creation_scenario() {
  local expected_file="$1"
  local scenario_id
  scenario_id="$(jq -r '.scenarioId' "${expected_file}")"

  echo "Running scenario: ${scenario_id}"
  local session_id
  session_id="$(create_session)"
  echo "  sessionId=${session_id}"
}

run_upload_flow_scenario() {
  local expected_file="$1"
  local scenario_id
  scenario_id="$(jq -r '.scenarioId' "${expected_file}")"

  local media_path
  local content_type
  media_path="$(jq -r '.request.mediaPath' "${expected_file}")"
  content_type="$(jq -r '.request.contentType' "${expected_file}")"

  if [ ! -f "${media_path}" ]; then
    fail "scenario ${scenario_id}: media path not found: ${media_path}"
  fi

  echo "Running scenario: ${scenario_id}"
  local session_id
  session_id="$(create_session)"

  local upload_out
  upload_out="$(upload_media "${session_id}" "${media_path}" "${content_type}")"
  local upload_id
  local job_id
  upload_id="${upload_out%%|*}"
  job_id="${upload_out##*|}"

  echo "  uploadId=${upload_id} jobId=${job_id}"

  local terminal_job_json
  terminal_job_json="$(poll_job_terminal "${session_id}" "${job_id}" "${scenario_id}")"
  assert_job_expectation "${expected_file}" "${terminal_job_json}"

  wait_for_session_pvp_queue_completion "${session_id}" "${expected_file}" "${scenario_id}"

  local pokemon_json
  pokemon_json="$(fetch_pokemon "${session_id}")"
  assert_pokemon_expectation "${expected_file}" "${pokemon_json}"

  echo "  scenario passed"
}

run_request_error_scenario() {
  local expected_file="$1"
  local scenario_id
  scenario_id="$(jq -r '.scenarioId' "${expected_file}")"

  local method
  local request_path
  local use_valid_session
  method="$(jq -r '.request.method' "${expected_file}")"
  request_path="$(jq -r '.request.path' "${expected_file}")"
  use_valid_session="$(jq -r '.request.useValidSession' "${expected_file}")"

  local status_expected
  local error_code_expected
  status_expected="$(jq -r '.apiExpectation.statusCode' "${expected_file}")"
  error_code_expected="$(jq -r '.apiExpectation.errorCode' "${expected_file}")"

  echo "Running scenario: ${scenario_id}"

  local header_args=()
  if [ "${use_valid_session}" = "true" ]; then
    local session_id
    session_id="$(create_session)"
    header_args=( -H "X-Session-Id: ${session_id}" )
  fi

  local body_file
  body_file="$(mktemp)"
  local status

  if jq -e '.request.multipartFile != null' "${expected_file}" >/dev/null; then
    local tmp_file
    tmp_file="$(mktemp)"
    local field
    local filename
    local content_type
    local content

    field="$(jq -r '.request.multipartFile.field' "${expected_file}")"
    filename="$(jq -r '.request.multipartFile.filename' "${expected_file}")"
    content_type="$(jq -r '.request.multipartFile.contentType' "${expected_file}")"
    content="$(jq -r '.request.multipartFile.content' "${expected_file}")"

    printf "%s" "${content}" > "${tmp_file}"

    status="$(curl -sS -o "${body_file}" -w "%{http_code}" -X "${method}" \
      ${header_args[@]+"${header_args[@]}"} \
      -F "${field}=@${tmp_file};filename=${filename};type=${content_type}" \
      "${WEB_BASE_URL}${request_path}")"
    rm -f "${tmp_file}"
  else
    status="$(curl -sS -o "${body_file}" -w "%{http_code}" -X "${method}" \
      ${header_args[@]+"${header_args[@]}"} \
      "${WEB_BASE_URL}${request_path}")"
  fi

  if [ "${status}" != "${status_expected}" ]; then
    cat "${body_file}" >&2
    rm -f "${body_file}"
    fail "scenario ${scenario_id}: expected HTTP ${status_expected}, got ${status}"
  fi

  local error_code_actual
  error_code_actual="$(jq -r '.error.code // empty' "${body_file}")"
  rm -f "${body_file}"

  if [ "${error_code_actual}" != "${error_code_expected}" ]; then
    fail "scenario ${scenario_id}: expected error code ${error_code_expected}, got ${error_code_actual}"
  fi

  echo "  scenario passed"
}

echo "Checking web health at ${WEB_BASE_URL}/healthz ..."
curl -fsS "${WEB_BASE_URL}/healthz" >/dev/null

verify_e2e_fixtures
reset_nonterminal_jobs_if_enabled
wait_for_idle_queue

echo "Running e2e smoke scenarios..."
while IFS= read -r expected_file; do
  scenario_type="$(jq -r '.type' "${expected_file}")"

  case "${scenario_type}" in
    session_creation)
      run_session_creation_scenario "${expected_file}"
      ;;
    upload_flow)
      run_upload_flow_scenario "${expected_file}"
      ;;
    request_error)
      run_request_error_scenario "${expected_file}"
      ;;
    *)
      fail "unsupported scenario type '${scenario_type}' in ${expected_file}"
      ;;
  esac

done < <(jq -r '.expectedFixtures[]' "${MANIFEST_PATH}")

echo "e2e smoke suite passed."
