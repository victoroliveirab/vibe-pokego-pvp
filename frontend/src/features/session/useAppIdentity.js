import { useAuth, useUser } from "@clerk/react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { createApiClient } from "../../lib/api-client";
import { clearStoredSessionId } from "./session-storage";

const defaultGuestApiClient = createApiClient({ identityMode: "guest" });

function createInitialGuestState() {
  return {
    sessionId: "",
    isLoading: true,
    error: null,
  };
}

function ownerKeyForClerkUser(userId) {
  return `clerk:${String(userId || "").trim()}`;
}

/**
 * @typedef {object} AppIdentity
 * @property {"guest"|"clerk"|""} mode
 * @property {string} sessionId
 * @property {boolean} isLoading
 * @property {Error|null} error
 * @property {boolean} isSignedIn
 * @property {import("@clerk/types").UserResource|null} user
 * @property {import("../../lib/api-client").ApiClient} apiClient
 * @property {function(): Promise<string>} bootstrapSession
 */

/**
 * @typedef {object} UseAppIdentityOptions
 * @property {import("../../lib/api-client").ApiClient} [guestApiClient]
 */

/**
 * Resolves the current application identity for both anonymous and signed-in users.
 *
 * @param {UseAppIdentityOptions} [options={}]
 * @returns {AppIdentity}
 */
export function useAppIdentity({ guestApiClient = defaultGuestApiClient } = {}) {
  const { getToken, isLoaded, isSignedIn, userId } = useAuth();
  const { user } = useUser();
  const [guestState, setGuestState] = useState(createInitialGuestState);

  const bootstrapSession = useCallback(async () => {
    setGuestState((previous) => ({
      ...previous,
      isLoading: true,
      error: null,
    }));

    try {
      const sessionId = await guestApiClient.ensureSession();
      setGuestState({
        sessionId,
        isLoading: false,
        error: null,
      });
      return sessionId;
    } catch (error) {
      setGuestState({
        sessionId: "",
        isLoading: false,
        error,
      });
      throw error;
    }
  }, [guestApiClient]);

  useEffect(() => {
    if (!isLoaded) {
      return;
    }

    if (isSignedIn) {
      clearStoredSessionId();
      setGuestState(createInitialGuestState());
      return;
    }

    let isMounted = true;

    const run = async () => {
      try {
        const sessionId = await guestApiClient.ensureSession();
        if (!isMounted) {
          return;
        }

        setGuestState({
          sessionId,
          isLoading: false,
          error: null,
        });
      } catch (error) {
        if (!isMounted) {
          return;
        }

        setGuestState({
          sessionId: "",
          isLoading: false,
          error,
        });
      }
    };

    void run();

    return () => {
      isMounted = false;
    };
  }, [guestApiClient, isLoaded, isSignedIn]);

  const clerkApiClient = useMemo(
    () =>
      createApiClient({
        identityMode: "clerk",
        authTokenProvider: async () => {
          const token = await getToken();
          return typeof token === "string" ? token : "";
        },
      }),
    [getToken],
  );

  if (!isLoaded) {
    return {
      mode: "",
      sessionId: "",
      isLoading: true,
      error: null,
      isSignedIn: false,
      user: null,
      apiClient: guestApiClient,
      bootstrapSession,
    };
  }

  if (isSignedIn) {
    return {
      mode: "clerk",
      sessionId: ownerKeyForClerkUser(userId),
      isLoading: false,
      error: null,
      isSignedIn: true,
      user: user || null,
      apiClient: clerkApiClient,
      bootstrapSession,
    };
  }

  return {
    mode: "guest",
    sessionId: guestState.sessionId,
    isLoading: guestState.isLoading,
    error: guestState.error,
    isSignedIn: false,
    user: null,
    apiClient: guestApiClient,
    bootstrapSession,
  };
}
