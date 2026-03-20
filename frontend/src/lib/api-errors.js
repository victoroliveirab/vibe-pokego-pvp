const fallbackErrorMessage = "Something went wrong while contacting the server.";

const errorMessageByCode = {
  INVALID_SESSION: "Your session expired. We can try again.",
  INTERNAL_ERROR: "Server error. Please try again.",
  MISSING_FILE: "Choose a file before submitting.",
  UNSUPPORTED_MEDIA_TYPE: "Only image and video files are supported.",
  VIDEO_TOO_LARGE: "Video must be 75 MB or smaller.",
  VIDEO_TOO_LONG: "Video must be 90 seconds or shorter.",
  JOB_NOT_FOUND: "This job is no longer available. Upload again to create a new job.",
  JOB_RETRY_NOT_ALLOWED: "Only failed jobs can be retried.",
};

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
 * @typedef {object} APIClientErrorOptions
 * @property {string} [code="API_ERROR"]
 * @property {string} [message=fallbackErrorMessage]
 * @property {number} [status=0]
 * @property {JsonValue|null} [details=null]
 */

/**
 * @typedef {object} APIErrorEnvelope
 * @property {string} [code]
 * @property {string} [message]
 * @property {JsonValue} [details]
 */

/**
 * Represents an HTTP or backend-level API failure normalized for the frontend.
 *
 * @extends Error
 */
export class APIClientError extends Error {
  /**
   * @param {APIClientErrorOptions} [options={}]
   */
  constructor({ code = "API_ERROR", message = fallbackErrorMessage, status = 0, details = null } = {}) {
    super(message);
    this.name = "APIClientError";
    this.code = code;
    this.status = status;
    this.details = details;
    this.userMessage = errorMessageByCode[code] || message || fallbackErrorMessage;
  }
}

/**
 * Extracts the backend error envelope from a parsed response payload.
 *
 * @param {JsonValue|null} payload
 * @returns {APIErrorEnvelope|null}
 */
function extractErrorEnvelope(payload) {
  if (!payload || typeof payload !== "object") {
    return null;
  }

  const candidate = payload.error;
  if (!candidate || typeof candidate !== "object") {
    return null;
  }

  return candidate;
}

/**
 * Parses an unsuccessful fetch response into an {@link APIClientError}.
 *
 * @param {Response} response
 * @returns {Promise<APIClientError>}
 */
export async function parseAPIError(response) {
  const rawBody = await response.text();

  let payload = null;
  if (rawBody) {
    try {
      payload = JSON.parse(rawBody);
    } catch (_error) {
      payload = null;
    }
  }

  const envelope = extractErrorEnvelope(payload);

  const code =
    envelope && typeof envelope.code === "string" && envelope.code.trim().length > 0
      ? envelope.code
      : `HTTP_${response.status}`;

  const message =
    envelope && typeof envelope.message === "string" && envelope.message.trim().length > 0
      ? envelope.message
      : response.statusText || fallbackErrorMessage;

  const details = envelope && Object.hasOwn(envelope, "details") ? envelope.details : null;

  return new APIClientError({
    code,
    message,
    status: response.status,
    details,
  });
}

/**
 * Returns the safest user-facing message available for an error.
 *
 * @param {APIClientError|Error|{ message?: string }|null|undefined} error
 * @returns {string}
 */
export function getUserFacingErrorMessage(error) {
  if (error instanceof APIClientError) {
    return error.userMessage;
  }

  if (error && typeof error === "object" && typeof error.message === "string" && error.message.trim()) {
    return error.message;
  }

  return fallbackErrorMessage;
}
