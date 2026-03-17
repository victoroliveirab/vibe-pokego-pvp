import { createApiClient } from "../../lib/api-client";
import { APIClientError, getUserFacingErrorMessage } from "../../lib/api-errors";

const defaultApiClient = createApiClient();

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

function normalizeOptionalString(value) {
  return typeof value === "string" ? value : "";
}

function normalizeOptionalNullableString(value) {
  if (value === null || value === undefined) {
    return null;
  }

  return normalizeOptionalString(value);
}

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

export function normalizeJobStatusError(error) {
  return normalizeJobErrorForUI(error, "JOB_STATUS_FAILED", "Failed to fetch job status");
}

export function normalizeRetryError(error) {
  return normalizeJobErrorForUI(error, "JOB_RETRY_FAILED", "Failed to retry job");
}

export function createJobApi({ apiClient = defaultApiClient } = {}) {
  return {
    async getActiveJob({ sessionId = "" } = {}) {
      const payload = await apiClient.request("/jobs/active", {
        method: "GET",
        requiresSession: true,
        sessionId,
      });

      if (!payload || typeof payload !== "object") {
        throw new APIClientError({
          code: "INVALID_RESPONSE",
          message: "Active job response payload was invalid.",
          details: payload,
        });
      }

      if (payload.job === null || payload.job === undefined) {
        return null;
      }

      return normalizeJobStatusResponse(payload.job);
    },

    async getJobStatus({ jobId, sessionId = "" }) {
      const normalizedJobId = normalizeRequiredString(jobId, "jobId", { jobId });
      const payload = await apiClient.request(`/jobs/${encodeURIComponent(normalizedJobId)}`, {
        method: "GET",
        requiresSession: true,
        sessionId,
      });

      return normalizeJobStatusResponse(payload);
    },

    async retryJob({ jobId, sessionId = "" }) {
      const normalizedJobId = normalizeRequiredString(jobId, "jobId", { jobId });
      const payload = await apiClient.request(`/jobs/${encodeURIComponent(normalizedJobId)}/retry`, {
        method: "POST",
        requiresSession: true,
        sessionId,
      });

      return normalizeRetryResponse(payload);
    },
  };
}
