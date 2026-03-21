import { createApiClient } from "../../lib/api-client";
import { APIClientError, getUserFacingErrorMessage } from "../../lib/api-errors";

const defaultApiClient = createApiClient();

/**
 * @typedef {null|boolean|number|string|JsonObject|JsonArray} JsonValue
 */

/**
 * @typedef {Object.<string, JsonValue>} JsonObject
 */

/**
 * @typedef {Array<JsonValue>} JsonArray
 */

/**
 * @typedef {object} JobApiClient
 * @property {function(string, { method?: string, headers?: HeadersInit, body?: BodyInit|null, requiresIdentity?: boolean, sessionId?: string }=): Promise<JsonValue|null>} request
 */

/**
 * @typedef {object} JobErrorPayload
 * @property {string} code
 * @property {string} message
 */

/**
 * @typedef {object} JobStatusResponse
 * @property {string} jobId
 * @property {string} uploadId
 * @property {string} status
 * @property {number} progress
 * @property {string|null} stage
 * @property {string} createdAt
 * @property {string} updatedAt
 * @property {string|null} finishedAt
 * @property {JobErrorPayload|null} error
 */

/**
 * @typedef {object} RetryJobResponse
 * @property {string} jobId
 * @property {string} parentJobId
 * @property {string} uploadId
 * @property {string} status
 */

/**
 * @typedef {object} NormalizedJobRequestError
 * @property {string} code
 * @property {string} message
 * @property {string} debugMessage
 * @property {JsonValue|null} details
 */

/**
 * @typedef {object} ActiveJobResponsePayload
 * @property {JsonObject|null} [job]
 */

/**
 * @typedef {object} JobRequestOptions
 * @property {string} [sessionId=""]
 */

/**
 * @typedef {object} GetJobStatusOptions
 * @property {string} jobId
 * @property {string} [sessionId=""]
 */

/**
 * @typedef {object} RetryJobOptions
 * @property {string} jobId
 * @property {string} [sessionId=""]
 */

/**
 * @typedef {object} JobApi
 * @property {function(JobRequestOptions=): Promise<JobStatusResponse|null>} getActiveJob
 * @property {function(GetJobStatusOptions): Promise<JobStatusResponse>} getJobStatus
 * @property {function(RetryJobOptions): Promise<RetryJobResponse>} retryJob
 */

/**
 * @typedef {object} CreateJobApiOptions
 * @property {JobApiClient} [apiClient]
 */

/**
 * Returns a required non-empty string field from a response payload.
 *
 * @param {unknown} value
 * @param {string} fieldName
 * @param {JsonValue|null} payload
 * @returns {string}
 * @throws {APIClientError}
 */
function normalizeRequiredString(value, fieldName, payload) {
  if (typeof value !== "string" || value.trim().length === 0) {
    throw new APIClientError({
      code: "INVALID_RESPONSE",
      message: `Job response was missing ${fieldName}.`,
      details: payload,
    });
  }

  return value;
}

/**
 * Normalizes an optional string field to either a string or an empty string.
 *
 * @param {unknown} value
 * @returns {string}
 */
function normalizeOptionalString(value) {
  return typeof value === "string" ? value : "";
}

/**
 * Normalizes a nullable string field to either a string or null.
 *
 * @param {unknown} value
 * @returns {string|null}
 */
function normalizeOptionalNullableString(value) {
  if (value === null || value === undefined) {
    return null;
  }

  return normalizeOptionalString(value);
}

/**
 * Normalizes the optional job error object embedded in job responses.
 *
 * @param {unknown} errorPayload
 * @param {JsonValue|null} payload
 * @returns {JobErrorPayload|null}
 * @throws {APIClientError}
 */
function normalizeJobError(errorPayload, payload) {
  if (errorPayload === null || errorPayload === undefined) {
    return null;
  }

  if (typeof errorPayload !== "object") {
    throw new APIClientError({
      code: "INVALID_RESPONSE",
      message: "Job response error field had an invalid shape.",
      details: payload,
    });
  }

  const code = normalizeOptionalString(errorPayload.code);
  const message = normalizeOptionalString(errorPayload.message);

  if (!code && !message) {
    return null;
  }

  return {
    code,
    message,
  };
}

/**
 * Normalizes job progress into a rounded percentage between 0 and 100.
 *
 * @param {unknown} progressValue
 * @param {JsonValue|null} payload
 * @returns {number}
 * @throws {APIClientError}
 */
function normalizeProgress(progressValue, payload) {
  if (typeof progressValue !== "number" || Number.isNaN(progressValue)) {
    throw new APIClientError({
      code: "INVALID_RESPONSE",
      message: "Job response was missing progress.",
      details: payload,
    });
  }

  return Math.max(0, Math.min(100, Math.round(progressValue)));
}

/**
 * Validates and normalizes a job status response payload.
 *
 * @param {JsonObject|null} payload
 * @returns {JobStatusResponse}
 * @throws {APIClientError}
 */
function normalizeJobStatusResponse(payload) {
  if (!payload || typeof payload !== "object") {
    throw new APIClientError({
      code: "INVALID_RESPONSE",
      message: "Job status response payload was invalid.",
      details: payload,
    });
  }

  return {
    jobId: normalizeRequiredString(payload.jobId, "jobId", payload),
    uploadId: normalizeRequiredString(payload.uploadId, "uploadId", payload),
    status: normalizeRequiredString(payload.status, "status", payload),
    progress: normalizeProgress(payload.progress, payload),
    stage: normalizeOptionalNullableString(payload.stage),
    createdAt: normalizeRequiredString(payload.createdAt, "createdAt", payload),
    updatedAt: normalizeRequiredString(payload.updatedAt, "updatedAt", payload),
    finishedAt: normalizeOptionalNullableString(payload.finishedAt),
    error: normalizeJobError(payload.error, payload),
  };
}

/**
 * Validates and normalizes a retry-job response payload.
 *
 * @param {JsonObject|null} payload
 * @returns {RetryJobResponse}
 * @throws {APIClientError}
 */
function normalizeRetryResponse(payload) {
  if (!payload || typeof payload !== "object") {
    throw new APIClientError({
      code: "INVALID_RESPONSE",
      message: "Retry response payload was invalid.",
      details: payload,
    });
  }

  return {
    jobId: normalizeRequiredString(payload.jobId, "jobId", payload),
    parentJobId: normalizeRequiredString(payload.parentJobId, "parentJobId", payload),
    uploadId: normalizeRequiredString(payload.uploadId, "uploadId", payload),
    status: normalizeRequiredString(payload.status, "status", payload),
  };
}

/**
 * Converts request failures into a consistent UI-facing job error shape.
 *
 * @param {APIClientError|Error|{ code?: string, message?: string, details?: JsonValue }|null|undefined} error
 * @param {string} fallbackCode
 * @param {string} fallbackDebugMessage
 * @returns {NormalizedJobRequestError}
 */
function normalizeJobErrorForUI(error, fallbackCode, fallbackDebugMessage) {
  const code = error && typeof error === "object" && typeof error.code === "string" ? error.code : fallbackCode;
  const debugMessage =
    error && typeof error === "object" && typeof error.message === "string" ? error.message : fallbackDebugMessage;
  const details =
    error && typeof error === "object" && Object.hasOwn(error, "details") && typeof error.details === "object"
      ? error.details
      : null;

  return {
    code,
    message: getUserFacingErrorMessage(error),
    debugMessage,
    details,
  };
}

/**
 * Normalizes a job status request failure for UI display.
 *
 * @param {APIClientError|Error|{ code?: string, message?: string, details?: JsonValue }|null|undefined} error
 * @returns {NormalizedJobRequestError}
 */
export function normalizeJobStatusError(error) {
  return normalizeJobErrorForUI(error, "JOB_STATUS_FAILED", "Failed to fetch job status");
}

/**
 * Normalizes a retry request failure for UI display.
 *
 * @param {APIClientError|Error|{ code?: string, message?: string, details?: JsonValue }|null|undefined} error
 * @returns {NormalizedJobRequestError}
 */
export function normalizeRetryError(error) {
  return normalizeJobErrorForUI(error, "JOB_RETRY_FAILED", "Failed to retry job");
}

/**
 * Creates the job API facade used by the upload workflow.
 *
 * @param {CreateJobApiOptions} [options={}]
 * @returns {JobApi}
 */
export function createJobApi({ apiClient = defaultApiClient } = {}) {
  return {
    /**
     * Fetches the active job for the current session, if one exists.
     *
     * @param {JobRequestOptions} [options={}]
     * @returns {Promise<JobStatusResponse|null>}
     * @throws {APIClientError}
     */
    async getActiveJob({ sessionId = "" } = {}) {
      const payload = await apiClient.request("/jobs/active", {
        method: "GET",
        requiresIdentity: true,
        sessionId,
      });

      /** @type {ActiveJobResponsePayload|null} */
      const activeJobPayload = payload && typeof payload === "object" && !Array.isArray(payload) ? payload : null;

      if (!activeJobPayload) {
        throw new APIClientError({
          code: "INVALID_RESPONSE",
          message: "Active job response payload was invalid.",
          details: payload,
        });
      }

      if (activeJobPayload.job === null || activeJobPayload.job === undefined) {
        return null;
      }

      return normalizeJobStatusResponse(activeJobPayload.job);
    },

    /**
     * Fetches and normalizes the current status for a specific job.
     *
     * @param {GetJobStatusOptions} options
     * @returns {Promise<JobStatusResponse>}
     * @throws {APIClientError}
     */
    async getJobStatus({ jobId, sessionId = "" }) {
      const normalizedJobId = normalizeRequiredString(jobId, "jobId", { jobId });
      const payload = await apiClient.request(`/jobs/${encodeURIComponent(normalizedJobId)}`, {
        method: "GET",
        requiresIdentity: true,
        sessionId,
      });

      return normalizeJobStatusResponse(payload);
    },

    /**
     * Requests a retry for a failed job and returns the replacement job metadata.
     *
     * @param {RetryJobOptions} options
     * @returns {Promise<RetryJobResponse>}
     * @throws {APIClientError}
     */
    async retryJob({ jobId, sessionId = "" }) {
      const normalizedJobId = normalizeRequiredString(jobId, "jobId", { jobId });
      const payload = await apiClient.request(`/jobs/${encodeURIComponent(normalizedJobId)}/retry`, {
        method: "POST",
        requiresIdentity: true,
        sessionId,
      });

      return normalizeRetryResponse(payload);
    },
  };
}
