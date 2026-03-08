# Runtime Configuration

This document defines runtime environment variables used by the local stack and service bootstrap.

## Required Variables

- `APP_ENV`: runtime environment name (`local` for local stack).
- `WEB_PORT`: web API listen port.
- `FRONTEND_PORT`: host port mapped to the frontend container.
- `WORKER_POLL_INTERVAL_SECS`: worker queue poll interval in seconds.
- `WORKER_HEALTH_PORT`: worker health endpoint listen port.
- `UPLOAD_STORAGE_MODE`: `local` or `uploadthing`.
- `UPLOADTHING_TOKEN`: required when `UPLOAD_STORAGE_MODE=uploadthing`.

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
2. Keep `APP_ENV=local` and `DATABASE_URL` unset to use local `DATABASE_PATH` fallback.
3. Keep `UPLOAD_STORAGE_MODE=local` for local tests unless explicitly validating UploadThing mode.
4. Start services with `make up`; startup fails fast when required variables are missing or invalid.
