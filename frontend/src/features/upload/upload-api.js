import { createApiClient } from "../../lib/api-client";
import { APIClientError, getUserFacingErrorMessage } from "../../lib/api-errors";

const defaultApiClient = createApiClient();

function normalizeUploadResponse(payload) {
  const uploadId = payload && typeof payload === "object" ? payload.uploadId : "";
  const jobId = payload && typeof payload === "object" ? payload.jobId : "";

  if (typeof uploadId !== "string" || uploadId.trim().length === 0) {
    throw new APIClientError({
      code: "INVALID_RESPONSE",
      message: "Upload response was missing uploadId.",
      details: payload,
    });
  }

  if (typeof jobId !== "string" || jobId.trim().length === 0) {
    throw new APIClientError({
      code: "INVALID_RESPONSE",
      message: "Upload response was missing jobId.",
      details: payload,
    });
  }

  return {
    uploadId,
    jobId,
  };
}

export function normalizeUploadError(error) {
  const code = error && typeof error === "object" && typeof error.code === "string" ? error.code : "UPLOAD_FAILED";
  const debugMessage =
    error && typeof error === "object" && typeof error.message === "string"
      ? error.message
      : "Upload request failed";

  return {
    code,
    message: getUserFacingErrorMessage(error),
    debugMessage,
  };
}

export function createUploadApi({ apiClient = defaultApiClient } = {}) {
  return {
    async submitUpload({ file, sessionId = "" }) {
      if (!file) {
        throw new APIClientError({
          code: "MISSING_FILE",
          message: "Missing required file field",
          status: 400,
        });
      }

      const formData = new FormData();
      formData.append("file", file);

      const payload = await apiClient.request("/uploads", {
        method: "POST",
        body: formData,
        requiresSession: true,
        sessionId,
      });

      return normalizeUploadResponse(payload);
    },
  };
}
