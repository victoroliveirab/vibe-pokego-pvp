import { useCallback, useEffect, useState } from "react";
import { createApiClient } from "../../lib/api-client";

const defaultApiClient = createApiClient();

/**
 * @typedef {object} AnonymousSessionApiClient
 * @property {function(): Promise<string>} ensureSession
 */

/**
 * @typedef {object} AnonymousSessionState
 * @property {string} sessionId
 * @property {boolean} isLoading
 * @property {Error|null} error
 */

/**
 * @typedef {object} UseAnonymousSessionOptions
 * @property {AnonymousSessionApiClient} [apiClient]
 */

/**
 * @typedef {AnonymousSessionState & {
 *   bootstrapSession: function(): Promise<string>
 * }} UseAnonymousSessionResult
 */

/**
 * Bootstraps and tracks the anonymous API session used by frontend features.
 *
 * @param {UseAnonymousSessionOptions} [options={}]
 * @returns {UseAnonymousSessionResult}
 */
export function useAnonymousSession({ apiClient = defaultApiClient } = {}) {
  const [state, setState] = useState({
    sessionId: "",
    isLoading: true,
    error: null,
  });

  const bootstrapSession = useCallback(async () => {
    setState((previous) => ({
      ...previous,
      isLoading: true,
      error: null,
    }));

    try {
      const sessionId = await apiClient.ensureSession();
      setState({
        sessionId,
        isLoading: false,
        error: null,
      });
      return sessionId;
    } catch (error) {
      setState({
        sessionId: "",
        isLoading: false,
        error,
      });
      throw error;
    }
  }, [apiClient]);

  useEffect(() => {
    let isMounted = true;

    const run = async () => {
      try {
        const sessionId = await apiClient.ensureSession();
        if (!isMounted) {
          return;
        }

        setState({
          sessionId,
          isLoading: false,
          error: null,
        });
      } catch (error) {
        if (!isMounted) {
          return;
        }

        setState({
          sessionId: "",
          isLoading: false,
          error,
        });
      }
    };

    run();

    return () => {
      isMounted = false;
    };
  }, [apiClient]);

  return {
    ...state,
    bootstrapSession,
  };
}
