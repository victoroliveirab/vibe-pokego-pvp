export const MAX_VIDEO_BYTES = 75 * 1024 * 1024;

function createValidationError(code, message) {
  return {
    code,
    message,
    debugMessage: message,
  };
}

function normalizeMimeType(type) {
  return typeof type === "string" ? type.trim().toLowerCase() : "";
}

export function validateSelectedUploadFile(file) {
  if (!file || typeof file !== "object") {
    return createValidationError("MISSING_FILE", "Choose a file before submitting.");
  }

  const mimeType = normalizeMimeType(file.type);
  const isImage = mimeType.startsWith("image/");
  const isVideo = mimeType.startsWith("video/");

  if (!isImage && !isVideo) {
    return createValidationError("UNSUPPORTED_MEDIA_TYPE", "Only image and video files are supported.");
  }

  const size = Number(file.size || 0);
  if (isVideo && size > MAX_VIDEO_BYTES) {
    return createValidationError("VIDEO_TOO_LARGE", "Video must be 75 MB or smaller.");
  }

  return null;
}
