/**
 * @typedef {object} StorageLike
 * @property {function(string): (string|null)} getItem
 * @property {function(string, string): void} setItem
 * @property {function(string): void} removeItem
 */

/** @type {string} */
export const SESSION_STORAGE_KEY = "pogo-anon-session-id-v1";

/**
 * Returns the default browser storage for anonymous session persistence.
 *
 * @returns {StorageLike|null}
 */
function getDefaultStorage() {
  if (typeof window === "undefined") {
    return null;
  }

  return window.localStorage;
}

/**
 * Reads the stored anonymous session ID if one exists.
 *
 * @param {StorageLike|null} [storage=getDefaultStorage()]
 * @returns {string|null}
 */
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

/**
 * Persists a non-empty anonymous session ID and returns the normalized value.
 *
 * @param {string} sessionId
 * @param {StorageLike|null} [storage=getDefaultStorage()]
 * @returns {string|null}
 * @throws {TypeError}
 */
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

/**
 * Removes any stored anonymous session ID.
 *
 * @param {StorageLike|null} [storage=getDefaultStorage()]
 * @returns {void}
 */
export function clearStoredSessionId(storage = getDefaultStorage()) {
  if (!storage) {
    return;
  }

  storage.removeItem(SESSION_STORAGE_KEY);
}
