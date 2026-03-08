import {
  clearStoredSessionId,
  getStoredSessionId,
  setStoredSessionId,
} from "../features/session/session-storage";
import { APIClientError, parseAPIError } from "./api-errors";

function getWebPort() {
  return (import.meta.env.WEB_PORT || import.meta.env.VITE_WEB_PORT || "8080").trim() || "8080";
}

function normalizeConfiguredBaseUrl(value) {
  const trimmed = (value || "").trim();
  if (!trimmed) {
    return "";
  }

  if (/^https?:\/\//i.test(trimmed)) {
    return trimmed.replace(/\/+$/, "");
  }

  const pathOnly = trimmed.replace(/^\/+/, "").replace(/\/+$/, "");
  if (!pathOnly) {
    return "";
  }

  return `/${pathOnly}`;
}

function getDefaultBaseUrl() {
  const configuredBaseUrl = normalizeConfiguredBaseUrl(import.meta.env.VITE_API_BASE_URL);
  if (configuredBaseUrl) {
    return configuredBaseUrl;
  }

  const webPort = getWebPort();

  if (typeof window !== "undefined" && window.location && window.location.hostname) {
    if (!import.meta.env.DEV && window.location.port === webPort) {
      return "";
    }

    const protocol = window.location.protocol === "https:" ? "https" : "http";
    return `${protocol}://${window.location.hostname}:${webPort}`;
  }

  if (import.meta.env.DEV) {
    return `http://127.0.0.1:${webPort}`;
  }

  return "";
}

const defaultBaseUrl = getDefaultBaseUrl();

function resolveUrl(baseUrl, path) {
  if (/^https?:\/\//.test(path)) {
    return path;
  }

  const normalizedBase = (baseUrl || "").replace(/\/+$/, "");
  const normalizedPath = path.startsWith("/") ? path : `/${path}`;

  if (!normalizedBase) {
    return normalizedPath;
  }

  return `${normalizedBase}${normalizedPath}`;
}

function createDefaultHeaders(headers) {
  const requestHeaders = new Headers(headers || {});
  if (!requestHeaders.has("Accept")) {
    requestHeaders.set("Accept", "application/json");
  }
  return requestHeaders;
}

async function parseSuccessResponse(response) {
  if (response.status === 204) {
    return null;
  }

  const text = await response.text();
  if (!text) {
    return null;
  }

  try {
    return JSON.parse(text);
  } catch (_error) {
    return text;
  }
}

export async function createSession(baseUrl = defaultBaseUrl, { fetchFn = fetch } = {}) {
  const response = await fetchFn(resolveUrl(baseUrl, "/session"), {
    method: "POST",
    headers: createDefaultHeaders(),
  });

  if (!response.ok) {
    throw await parseAPIError(response);
  }

  const payload = await parseSuccessResponse(response);
  const sessionId = payload && typeof payload === "object" ? payload.sessionId : "";

  if (typeof sessionId !== "string" || sessionId.trim().length === 0) {
    throw new APIClientError({
      code: "INVALID_RESPONSE",
      message: "Session response was missing sessionId.",
      status: response.status,
      details: payload,
    });
  }

  return sessionId;
}

export function createApiClient({
  baseUrl = defaultBaseUrl,
  fetchFn = fetch,
  sessionStorage = {
    get: getStoredSessionId,
    set: setStoredSessionId,
    clear: clearStoredSessionId,
  },
} = {}) {
  let inFlightSessionPromise = null;

  async function createAndPersistSession() {
    const freshSessionId = await createSession(baseUrl, { fetchFn });
    sessionStorage.set(freshSessionId);
    return freshSessionId;
  }

  async function bootstrapSession() {
    const existingSessionId = sessionStorage.get();
    if (existingSessionId) {
      return existingSessionId;
    }

    if (!inFlightSessionPromise) {
      inFlightSessionPromise = createAndPersistSession();
    }

    try {
      return await inFlightSessionPromise;
    } finally {
      inFlightSessionPromise = null;
    }
  }

  async function request(
    path,
    { method = "GET", headers, body, requiresSession = false, sessionId: preferredSessionId = "" } = {},
  ) {
    async function doRequest(sessionId) {
      const requestHeaders = createDefaultHeaders(headers);
      if (requiresSession && sessionId) {
        requestHeaders.set("X-Session-Id", sessionId);
      }

      const response = await fetchFn(resolveUrl(baseUrl, path), {
        method,
        headers: requestHeaders,
        body,
      });

      if (!response.ok) {
        throw await parseAPIError(response);
      }

      return parseSuccessResponse(response);
    }

    let sessionId = "";
    if (requiresSession) {
      sessionId = preferredSessionId || (await bootstrapSession());
    }

    try {
      return await doRequest(sessionId);
    } catch (error) {
      if (requiresSession && error instanceof APIClientError && error.code === "INVALID_SESSION") {
        sessionStorage.clear();
        const refreshedSessionId = await bootstrapSession();
        return doRequest(refreshedSessionId);
      }
      throw error;
    }
  }

  return {
    createSession: () => createSession(baseUrl, { fetchFn }),
    ensureSession: bootstrapSession,
    request,
  };
}
