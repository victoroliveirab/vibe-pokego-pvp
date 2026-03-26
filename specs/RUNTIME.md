# Runtime Configuration

This document defines runtime environment variables used by the local stack and service bootstrap.

## Required Variables

- `APP_ENV`: runtime environment name (`local` for local stack).
- `WEB_PORT`: web API listen port.
- `FRONTEND_PORT`: host port mapped to the frontend container.
- `VITE_DEPLOYED_AT_ISO`: ISO-8601 deployment timestamp baked into production frontend image builds and displayed in the shared footer.
- `WORKER_POLL_INTERVAL_SECS`: worker queue poll interval in seconds.
- `WORKER_HEALTH_PORT`: worker health endpoint listen port.
- `UPLOAD_STORAGE_MODE`: `local` or `uploadthing`.
- `UPLOADTHING_TOKEN`: required when `UPLOAD_STORAGE_MODE=uploadthing`.

## Clerk Variable Matrix

### `CLERK_ENABLED=false`

- Clerk settings are optional and ignored by the web API.

### `CLERK_ENABLED=true`

- `CLERK_SECRET_KEY` is required.
- `CLERK_AUTHORIZED_PARTIES` is required and should contain the frontend origins allowed to mint bearer tokens.
- `CLERK_PROXY_URL` is optional and enables same-origin Clerk Frontend API proxying (for example `/api/__clerk`).
- `CLERK_JWKS_URL` is optional and, when set, must be a valid URL.
- `CLERK_FRONTEND_API_URL` is optional and defaults to `https://frontend-api.clerk.dev` when proxying is enabled.
- `VITE_CLERK_PUBLISHABLE_KEY` is required by the frontend runtime even though it is consumed outside the Go service.
- `VITE_DEPLOYED_AT_ISO` is required for production-style frontend image builds and should be set to the deployment timestamp in ISO-8601 format.

## Database Variable Matrix

### Local mode (`APP_ENV=local`)

- `DATABASE_URL` may be unset.
- `DATABASE_PATH` is required when `DATABASE_URL` is unset.
- Services build a local DSN from `DATABASE_PATH` (for example `file:./var/app.db`).
- `DATABASE_AUTH_TOKEN` must be unset for local database connections.

### Deploy-like mode (`APP_ENV!=local`)

- `DATABASE_URL` is required and should point to the remote libsql/Turso database.
- `DATABASE_AUTH_TOKEN` is required for remote `DATABASE_URL` values.
- `DATABASE_PATH` is optional and ignored by remote runtime paths.

## Storage Variable Matrix

### `UPLOAD_STORAGE_MODE=local`

- `UPLOAD_LOCAL_DIR` is required.
- Services trim and validate the path, create the directory when missing, and fail startup if it is not writable.
- Recommended path is `./testdata/uploads` for deterministic smoke and fixture behavior.

### `UPLOAD_STORAGE_MODE=uploadthing`

- `UPLOADTHING_TOKEN` is required.
- `UPLOADTHING_PREPARE_UPLOAD_URL` is optional (defaults to `https://api.uploadthing.com/v7/prepareUpload`).
- `UPLOADTHING_REQUEST_TIMEOUT_SECS` must be a positive integer (defaults to `30`).
- `UPLOAD_LOCAL_DIR` is optional and may be unset.
- Worker download controls:
  - `WORKER_UPLOADTHING_DOWNLOAD_TIMEOUT_SECS` must be a positive integer (defaults to `30`).
  - `WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT` must be zero or greater (defaults to `2`).
  - `WORKER_UPLOADTHING_DOWNLOAD_TEMP_DIR` is optional; when set it must be writable.

## Storage Modes

### `UPLOAD_STORAGE_MODE=uploadthing`

- Intended deployment mode.
- Media binaries are stored via UploadThing.
- `UPLOAD_LOCAL_DIR` may be unset.
- Web upload endpoint requires `UPLOADTHING_TOKEN` (and optional endpoint/timeout overrides).
- Worker downloads persisted UploadThing URLs to temporary local files before decode/OCR.

### `UPLOAD_STORAGE_MODE=local`

- Local development and deterministic test mode.
- Services validate `UPLOAD_LOCAL_DIR`, create it when missing, and fail startup if it is not writable.
- Recommended path is `./testdata/uploads` for consistency with test fixtures and smoke flows.

## Fail-Fast Examples

- Missing URL in deploy-like mode:
  - `DATABASE_URL is required when APP_ENV is not local`
- Missing path for local fallback mode:
  - `DATABASE_PATH is required when DATABASE_URL is not set in local mode`
- Missing remote auth token:
  - `DATABASE_AUTH_TOKEN is required for remote DATABASE_URL`
- Local mode with auth token set:
  - `DATABASE_AUTH_TOKEN must be empty for local database connections`
- Missing local dir in local mode:
  - `UPLOAD_LOCAL_DIR is required when UPLOAD_STORAGE_MODE=local`
- Missing uploadthing token in uploadthing mode:
  - `UPLOADTHING_TOKEN is required when UPLOAD_STORAGE_MODE=uploadthing`
- Missing Clerk secret key when enabled:
  - `CLERK_SECRET_KEY is required when CLERK_ENABLED=true`
- Missing Clerk authorized parties when enabled:
  - `CLERK_AUTHORIZED_PARTIES is required when CLERK_ENABLED=true`
- Invalid Clerk JWKS URL:
  - `CLERK_JWKS_URL must be a valid URL`
- Invalid uploadthing timeout:
  - `UPLOADTHING_REQUEST_TIMEOUT_SECS must be a positive integer`
- Invalid worker uploadthing retry count:
  - `WORKER_UPLOADTHING_DOWNLOAD_RETRY_COUNT must be zero or greater`
- Unsupported storage mode:
  - `UPLOAD_STORAGE_MODE must be one of: local, uploadthing`
- Unwritable local dir:
  - `UPLOAD_LOCAL_DIR must be writable: <system error>`

## Local Workflow

1. Copy `.env.example` to `.env`.
2. Set real Clerk development keys in `VITE_CLERK_PUBLISHABLE_KEY` and `CLERK_SECRET_KEY`, then keep `CLERK_ENABLED=true` if you want local auth behavior to match production.
3. Keep `CLERK_AUTHORIZED_PARTIES` aligned with the frontend origins you use locally.
4. Keep `APP_ENV=local` and `DATABASE_URL` unset to use local `DATABASE_PATH` fallback.
5. Keep `UPLOAD_STORAGE_MODE=local` for local tests unless explicitly validating UploadThing mode.
6. Start services with `make up`; startup fails fast when required variables are missing or invalid.

## Auth Mode Behavior

- Guest mode uses `POST /session` plus `X-Session-Id`.
- Clerk mode uses `Authorization: Bearer <session token>` on the same protected endpoints.
- Guest records are intentionally temporary.
- Signing in clears the locally stored guest session and resets guest-bound UI state before authenticated data reloads.
- Authenticated records are stored by Clerk owner key and are intended to persist across devices.

## Smoke Coverage

- `scripts/smoke/verify_stack.sh` and `scripts/smoke/e2e.sh` validate the deterministic guest flow.
- Authenticated Clerk smoke is not automated in this repository because it would depend on live browser session token acquisition and would be brittle in CI/local shells.
- Clerk behavior is instead covered by mocked unit/integration tests plus manual browser verification.
