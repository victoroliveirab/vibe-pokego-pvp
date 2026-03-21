import { renderHook, waitFor } from "@testing-library/react";
import { useAppIdentity } from "./useAppIdentity";

const clerkState = vi.hoisted(() => ({
  auth: {
    isLoaded: true,
    isSignedIn: false,
    userId: null,
    getToken: vi.fn().mockResolvedValue(""),
  },
  user: {
    user: null,
  },
}));

vi.mock("@clerk/react", () => ({
  useAuth: () => clerkState.auth,
  useUser: () => clerkState.user,
}));

describe("useAppIdentity", () => {
  beforeEach(() => {
    window.localStorage.clear();
    clerkState.auth.isLoaded = true;
    clerkState.auth.isSignedIn = false;
    clerkState.auth.userId = null;
    clerkState.auth.getToken = vi.fn().mockResolvedValue("");
    clerkState.user.user = null;
  });

  it("bootstraps a guest session when signed out", async () => {
    const guestApiClient = {
      ensureSession: vi.fn().mockResolvedValue("session-1"),
    };

    const { result } = renderHook(() => useAppIdentity({ guestApiClient }));

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.mode).toBe("guest");
    expect(result.current.sessionId).toBe("session-1");
    expect(guestApiClient.ensureSession).toHaveBeenCalledTimes(1);
  });

  it("returns clerk identity without bootstrapping a guest session", async () => {
    window.localStorage.setItem("pogo-anon-session-id-v1", "session-1");
    clerkState.auth.isSignedIn = true;
    clerkState.auth.userId = "user_123";
    clerkState.auth.getToken = vi.fn().mockResolvedValue("token-123");
    clerkState.user.user = { id: "user_123", fullName: "Test User" };

    const guestApiClient = {
      ensureSession: vi.fn().mockResolvedValue("session-1"),
    };

    const { result } = renderHook(() => useAppIdentity({ guestApiClient }));

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.mode).toBe("clerk");
    expect(result.current.sessionId).toBe("clerk:user_123");
    expect(result.current.isSignedIn).toBe(true);
    expect(result.current.user).toEqual(clerkState.user.user);
    expect(guestApiClient.ensureSession).not.toHaveBeenCalled();
    expect(window.localStorage.getItem("pogo-anon-session-id-v1")).toBeNull();
  });

  it("bootstraps a fresh guest session after sign-out", async () => {
    clerkState.auth.isSignedIn = true;
    clerkState.auth.userId = "user_123";
    clerkState.user.user = { id: "user_123" };

    const guestApiClient = {
      ensureSession: vi.fn().mockResolvedValue("session-2"),
    };

    const { result, rerender } = renderHook(() => useAppIdentity({ guestApiClient }));

    await waitFor(() => {
      expect(result.current.mode).toBe("clerk");
    });

    clerkState.auth.isSignedIn = false;
    clerkState.auth.userId = null;
    clerkState.user.user = null;
    rerender();

    await waitFor(() => {
      expect(result.current.mode).toBe("guest");
      expect(result.current.sessionId).toBe("session-2");
      expect(result.current.isLoading).toBe(false);
    });

    expect(guestApiClient.ensureSession).toHaveBeenCalledTimes(1);
  });
});
