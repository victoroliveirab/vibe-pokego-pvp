export const SESSION_STORAGE_KEY = "pogo-anon-session-id-v1";

function getDefaultStorage() {
  if (typeof window === "undefined") {
    return null;
  }

  return window.localStorage;
}

export function getStoredSessionId(storage = getDefaultStorage()) {
  if (!storage) {
    return null;
  }

  const sessionId = storage.getItem(SESSION_STORAGE_KEY);
  if (typeof sessionId !== "string") {
    return null;
  }

  const normalized = sessionId.trim();
  return normalized.length > 0 ? normalized : null;
}

export function setStoredSessionId(sessionId, storage = getDefaultStorage()) {
  if (!storage) {
    return null;
  }

  const normalized = String(sessionId || "").trim();
  if (!normalized) {
    throw new TypeError("sessionId must be a non-empty string");
  }

  storage.setItem(SESSION_STORAGE_KEY, normalized);
  return normalized;
}

export function clearStoredSessionId(storage = getDefaultStorage()) {
  if (!storage) {
    return;
  }

  storage.removeItem(SESSION_STORAGE_KEY);
}
