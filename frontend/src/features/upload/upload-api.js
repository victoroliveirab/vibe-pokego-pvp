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
 * @typedef {object} UploadApiClient
 * @property {function(string, { method?: string, headers?: HeadersInit, body?: BodyInit|null, requiresIdentity?: boolean, sessionId?: string }=): Promise<JsonValue|null>} request
 */

/**
 * @typedef {object} UploadResponsePayload
 * @property {string} uploadId
 * @property {string} jobId
 */

/**
 * @typedef {object} NormalizedUploadResponse
 * @property {string} uploadId
 * @property {string} jobId
 */

/**
 * @typedef {object} NormalizedUploadError
 * @property {string} code
 * @property {string} message
 * @property {string} debugMessage
 */

/**
 * @typedef {object} SubmitUploadOptions
 * @property {File} file
 * @property {string} [sessionId=""]
 */

/**
 * @typedef {object} UploadApi
 * @property {function(SubmitUploadOptions): Promise<NormalizedUploadResponse>} submitUpload
 */

/**
 * @typedef {object} CreateUploadApiOptions
 * @property {UploadApiClient} [apiClient]
 */

/**
 * Validates and normalizes the upload creation payload returned by the backend.
 *
 * @param {JsonValue|null} payload
 * @returns {NormalizedUploadResponse}
 * @throws {APIClientError}
 */
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

/**
 * Converts an upload request failure into a UI-friendly error object.
 *
 * @param {APIClientError|Error|{ code?: string, message?: string }|null|undefined} error
 * @returns {NormalizedUploadError}
 */
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

/**
 * Creates the upload API facade used by the upload workflow.
 *
 * @param {CreateUploadApiOptions} [options={}]
 * @returns {UploadApi}
 */
export function createUploadApi({ apiClient = defaultApiClient } = {}) {
  return {
    /**
     * Submits a file upload and returns the normalized upload and job IDs.
     *
     * @param {SubmitUploadOptions} options
     * @returns {Promise<NormalizedUploadResponse>}
     * @throws {APIClientError}
     */
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
        requiresIdentity: true,
        sessionId,
      });

      return normalizeUploadResponse(payload);
    },
  };
}
