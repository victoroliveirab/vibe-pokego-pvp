import { createApiClient, createSession } from "./api-client";
import { APIClientError } from "./api-errors";

function jsonResponse(status, payload) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "Content-Type": "application/json",
    },
  });
}

describe("api client", () => {
  it("creates a session with POST /session", async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(201, { sessionId: "session-1" }));

    const sessionId = await createSession("http://localhost:8080", { fetchFn });

    expect(sessionId).toBe("session-1");
    expect(fetchFn).toHaveBeenCalledWith(
      "http://localhost:8080/session",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("creates a session with same-origin /api base", async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(201, { sessionId: "session-1" }));

    const sessionId = await createSession("/api", { fetchFn });

    expect(sessionId).toBe("session-1");
    expect(fetchFn).toHaveBeenCalledWith(
      "/api/session",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("normalizes trailing slash for /api base", async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(201, { sessionId: "session-1" }));

    const sessionId = await createSession("/api/", { fetchFn });

    expect(sessionId).toBe("session-1");
    expect(fetchFn).toHaveBeenCalledWith(
      "/api/session",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("throws when /session response is missing sessionId", async () => {
    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(201, { ok: true }));

    await expect(createSession("", { fetchFn })).rejects.toMatchObject({ code: "INVALID_RESPONSE" });
  });

  it("reuses stored session for protected requests", async () => {
    const sessionStorage = {
      get: vi.fn(() => "persisted-session"),
      set: vi.fn(),
      clear: vi.fn(),
    };

    const fetchFn = vi.fn().mockResolvedValue(jsonResponse(200, { ok: true }));

    const apiClient = createApiClient({
      baseUrl: "http://localhost:8080",
      fetchFn,
      sessionStorage,
    });

    await apiClient.request("/protected/ping", {
      method: "GET",
      requiresSession: true,
    });

    const requestConfig = fetchFn.mock.calls[0][1];
    expect(requestConfig.headers.get("X-Session-Id")).toBe("persisted-session");
    expect(sessionStorage.set).not.toHaveBeenCalled();
  });

  it("recovers from INVALID_SESSION by minting a new session and retrying once", async () => {
    const storageState = {
      value: "expired-session",
    };

    const sessionStorage = {
      get: vi.fn(() => storageState.value),
      set: vi.fn((sessionId) => {
        storageState.value = sessionId;
      }),
      clear: vi.fn(() => {
        storageState.value = "";
      }),
    };

    const fetchFn = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse(401, {
          error: {
            code: "INVALID_SESSION",
            message: "Missing or invalid X-Session-Id",
          },
        }),
      )
      .mockResolvedValueOnce(jsonResponse(201, { sessionId: "fresh-session" }))
      .mockResolvedValueOnce(jsonResponse(201, { uploadId: "upload-1", jobId: "job-1" }));

    const apiClient = createApiClient({
      baseUrl: "http://localhost:8080",
      fetchFn,
      sessionStorage,
    });

    const response = await apiClient.request("/uploads", {
      method: "POST",
      requiresSession: true,
    });

    expect(response).toEqual({ uploadId: "upload-1", jobId: "job-1" });
    expect(sessionStorage.clear).toHaveBeenCalledTimes(1);
    expect(sessionStorage.set).toHaveBeenCalledWith("fresh-session");

    const firstProtectedCallHeaders = fetchFn.mock.calls[0][1].headers;
    const retriedProtectedCallHeaders = fetchFn.mock.calls[2][1].headers;
    expect(firstProtectedCallHeaders.get("X-Session-Id")).toBe("expired-session");
    expect(retriedProtectedCallHeaders.get("X-Session-Id")).toBe("fresh-session");
  });

  it("normalizes API envelope errors", async () => {
    const sessionStorage = {
      get: vi.fn(() => "session-1"),
      set: vi.fn(),
      clear: vi.fn(),
    };

    const fetchFn = vi.fn().mockResolvedValue(
      jsonResponse(415, {
        error: {
          code: "UNSUPPORTED_MEDIA_TYPE",
          message: "Only image and video uploads are supported",
        },
      }),
    );

    const apiClient = createApiClient({
      baseUrl: "",
      fetchFn,
      sessionStorage,
    });

    await expect(
      apiClient.request("/uploads", {
        method: "POST",
        requiresSession: true,
      }),
    ).rejects.toMatchObject({
      code: "UNSUPPORTED_MEDIA_TYPE",
      userMessage: "Only image and video files are supported.",
    });
  });

  it("propagates APIClientError for server failures", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      jsonResponse(500, {
        error: {
          code: "INTERNAL_ERROR",
          message: "Internal server error",
        },
      }),
    );

    await expect(createSession("", { fetchFn })).rejects.toBeInstanceOf(APIClientError);
  });

  it("deduplicates concurrent session bootstrap requests", async () => {
    const storageState = {
      value: "",
    };

    const sessionStorage = {
      get: vi.fn(() => storageState.value),
      set: vi.fn((sessionId) => {
        storageState.value = sessionId;
      }),
      clear: vi.fn(() => {
        storageState.value = "";
      }),
    };

    let resolveSessionResponse;
    const sessionResponsePromise = new Promise((resolve) => {
      resolveSessionResponse = resolve;
    });

    const fetchFn = vi.fn().mockImplementation((url) => {
      if (url.endsWith("/session")) {
        return sessionResponsePromise;
      }
      return Promise.resolve(jsonResponse(200, { ok: true }));
    });

    const apiClient = createApiClient({
      baseUrl: "http://localhost:8080",
      fetchFn,
      sessionStorage,
    });

    const firstRequest = apiClient.request("/protected/ping", {
      method: "GET",
      requiresSession: true,
    });
    const secondRequest = apiClient.request("/protected/ping", {
      method: "GET",
      requiresSession: true,
    });

    resolveSessionResponse(jsonResponse(201, { sessionId: "session-1" }));

    await Promise.all([firstRequest, secondRequest]);

    const sessionCalls = fetchFn.mock.calls.filter(([url]) => url.endsWith("/session"));
    expect(sessionCalls).toHaveLength(1);
    expect(sessionStorage.set).toHaveBeenCalledWith("session-1");
  });
});
