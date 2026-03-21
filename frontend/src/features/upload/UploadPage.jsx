import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from "react";
import { getUserFacingErrorMessage } from "../../lib/api-errors";
import { createPokemonResultsApi } from "../results/pokemon-results-api";
import PokemonResultsPanel from "../results/PokemonResultsPanel";
import { initialPokemonResultsState, pokemonResultsStateReducer } from "../results/pokemon-results-state";
import { useAppIdentity } from "../session/useAppIdentity";
import { createJobApi, normalizeJobStatusError, normalizeRetryError } from "./job-api";
import UploadForm from "./UploadForm";
import { createUploadApi, normalizeUploadError } from "./upload-api";
import UploadStatusPanel from "./UploadStatusPanel";
import { initialUploadState, uploadFlowPhases, uploadStateReducer } from "./upload-state";
import { validateSelectedUploadFile } from "./upload-validation";

const terminalJobStatuses = new Set(["SUCCEEDED", "FAILED", "PENDING_USER_DEDUP"]);
const pokemonResultsRefreshStatuses = new Set(["SUCCEEDED", "PENDING_USER_DEDUP"]);
const initialDeleteModalState = {
  isOpen: false,
  resultId: "",
  speciesName: "",
};

function isTerminalJobStatus(status) {
  return terminalJobStatuses.has(status);
}

function phaseLabel(phase) {
  switch (phase) {
    case uploadFlowPhases.SESSION_LOADING:
      return "Preparing Upload";
    case uploadFlowPhases.READY:
      return "Ready to Upload";
    case uploadFlowPhases.UPLOADING:
      return "Upload in Progress";
    case uploadFlowPhases.SUCCESS:
      return "Upload Received";
    case uploadFlowPhases.ERROR:
      return "Please Review";
    default:
      return "Getting Started";
  }
}

function normalizePokemonResultsError(error) {
  const code =
    error && typeof error === "object" && typeof error.code === "string" ? error.code : "POKEMON_RESULTS_FAILED";
  const debugMessage =
    error && typeof error === "object" && typeof error.message === "string"
      ? error.message
      : "Failed to fetch pokemon results";

  return {
    code,
    message: getUserFacingErrorMessage(error),
    debugMessage,
  };
}

export default function UploadPage({
  uploadApi: injectedUploadApi = null,
  jobApi: injectedJobApi = null,
  monitorIntervalMs = 2000,
  pokemonResultsApi: injectedPokemonResultsApi = null,
  useSessionHook = useAppIdentity,
}) {
  const identity = useSessionHook();
  const uploadApi = useMemo(
    () => injectedUploadApi || createUploadApi({ apiClient: identity.apiClient }),
    [identity.apiClient, injectedUploadApi],
  );
  const jobApi = useMemo(
    () => injectedJobApi || createJobApi({ apiClient: identity.apiClient }),
    [identity.apiClient, injectedJobApi],
  );
  const pokemonResultsApi = useMemo(
    () => injectedPokemonResultsApi || createPokemonResultsApi({ apiClient: identity.apiClient }),
    [identity.apiClient, injectedPokemonResultsApi],
  );
  const [state, dispatch] = useReducer(uploadStateReducer, initialUploadState);
  const [pokemonResultsState, dispatchPokemonResults] = useReducer(
    pokemonResultsStateReducer,
    initialPokemonResultsState,
  );
  const [isDebugMode, setIsDebugMode] = useState(false);
  const [resolvingReadingIds, setResolvingReadingIds] = useState([]);
  const [pendingResolveError, setPendingResolveError] = useState(null);
  const [deletingResultIds, setDeletingResultIds] = useState([]);
  const [pendingDeleteError, setPendingDeleteError] = useState(null);
  const [deleteModalState, setDeleteModalState] = useState(initialDeleteModalState);
  const { sessionId, isLoading, error: sessionError } = identity;
  const pokemonResultsRequestIdRef = useRef(0);
  const lastSessionPokemonFetchRef = useRef("");
  const lastJobPokemonFetchKeyRef = useRef("");
  const activeJobBootstrapSessionRef = useRef("");
  const previousIdentityModeRef = useRef(identity.mode);

  useEffect(() => {
    const previousIdentityMode = previousIdentityModeRef.current;

    if (previousIdentityMode && previousIdentityMode !== identity.mode) {
      pokemonResultsRequestIdRef.current += 1;
      lastSessionPokemonFetchRef.current = "";
      lastJobPokemonFetchKeyRef.current = "";
      activeJobBootstrapSessionRef.current = "";
      setResolvingReadingIds([]);
      setPendingResolveError(null);
      setDeletingResultIds([]);
      setPendingDeleteError(null);
      setDeleteModalState(initialDeleteModalState);
      dispatch({ type: "reset" });
      dispatchPokemonResults({ type: "reset" });
    }

    previousIdentityModeRef.current = identity.mode;
  }, [identity.mode]);

  const refreshPokemonResults = useCallback(
    async ({ sessionId: sourceSessionId, preserveItems = true } = {}) => {
      const normalizedSessionId = typeof sourceSessionId === "string" ? sourceSessionId.trim() : "";
      if (!normalizedSessionId) {
        return;
      }

      pokemonResultsRequestIdRef.current += 1;
      const requestId = pokemonResultsRequestIdRef.current;
      dispatchPokemonResults({ type: "request-started", preserveItems });

      try {
        const [resultsPayload, pendingPayload] = await Promise.all([
          pokemonResultsApi.getPokemonResults({
            sessionId: normalizedSessionId,
          }),
          pokemonResultsApi.getPendingSpeciesReadings({
            sessionId: normalizedSessionId,
          }),
        ]);

        if (requestId !== pokemonResultsRequestIdRef.current) {
          return;
        }

        setPendingResolveError(null);
        dispatchPokemonResults({
          type: "request-succeeded",
          items: resultsPayload && Array.isArray(resultsPayload.results) ? resultsPayload.results : [],
          pendingItems: pendingPayload && Array.isArray(pendingPayload.readings) ? pendingPayload.readings : [],
          fetchedAt: new Date().toISOString(),
        });
      } catch (error) {
        if (requestId !== pokemonResultsRequestIdRef.current) {
          return;
        }

        dispatchPokemonResults({
          type: "request-failed",
          error: normalizePokemonResultsError(error),
        });
      }
    },
    [pokemonResultsApi],
  );

  useEffect(() => {
    if (isLoading) {
      dispatch({ type: "set-phase", phase: uploadFlowPhases.SESSION_LOADING });
      return;
    }

    if (sessionError) {
      dispatch({
        type: "set-error",
        error: {
          code: sessionError.code || "SESSION_BOOTSTRAP_FAILED",
          message: getUserFacingErrorMessage(sessionError),
          debugMessage: sessionError.message || "Session bootstrap failed",
        },
      });
      return;
    }

    if (sessionId) {
      dispatch({ type: "set-session", sessionId });
    }
  }, [isLoading, sessionError, sessionId]);

  useEffect(() => {
    const normalizedSessionId = typeof state.sessionId === "string" ? state.sessionId.trim() : "";

    if (!normalizedSessionId) {
      pokemonResultsRequestIdRef.current += 1;
      lastSessionPokemonFetchRef.current = "";
      lastJobPokemonFetchKeyRef.current = "";
      setResolvingReadingIds([]);
      setPendingResolveError(null);
      setDeletingResultIds([]);
      setPendingDeleteError(null);
      setDeleteModalState(initialDeleteModalState);
      dispatchPokemonResults({ type: "reset" });
      return;
    }

    if (lastSessionPokemonFetchRef.current === normalizedSessionId) {
      return;
    }

    lastSessionPokemonFetchRef.current = normalizedSessionId;
    lastJobPokemonFetchKeyRef.current = "";
    void refreshPokemonResults({
      sessionId: normalizedSessionId,
      preserveItems: false,
    });
  }, [refreshPokemonResults, state.sessionId]);

  useEffect(() => {
    const normalizedSessionId = typeof state.sessionId === "string" ? state.sessionId.trim() : "";
    const getActiveJob = typeof jobApi.getActiveJob === "function" ? jobApi.getActiveJob.bind(jobApi) : null;

    if (!normalizedSessionId || !getActiveJob) {
      activeJobBootstrapSessionRef.current = "";
      return;
    }

    if (activeJobBootstrapSessionRef.current === normalizedSessionId) {
      return;
    }

    activeJobBootstrapSessionRef.current = normalizedSessionId;
    let isDisposed = false;

    const bootstrapActiveJob = async () => {
      try {
        const activeJob = await getActiveJob({
          sessionId: normalizedSessionId,
        });

        if (isDisposed || !activeJob) {
          return;
        }

        dispatch({
          type: "resume-active-job",
          job: activeJob,
          polledAt: new Date().toISOString(),
        });
      } catch (_error) {
        if (isDisposed) {
          return;
        }
      }
    };

    void bootstrapActiveJob();

    return () => {
      isDisposed = true;
    };
  }, [jobApi, state.sessionId]);

  useEffect(() => {
    if (state.phase !== uploadFlowPhases.SUCCESS || !state.sessionId || !state.jobId) {
      return undefined;
    }

    if (isTerminalJobStatus(state.jobStatus)) {
      return undefined;
    }

    let isDisposed = false;
    let isRequestInFlight = false;
    let timeoutId = null;
    const activeJobId = state.jobId;
    const activeSessionId = state.sessionId;

    const pollJobStatus = async () => {
      if (isDisposed || isRequestInFlight) {
        return;
      }

      isRequestInFlight = true;
      let shouldContinuePolling = true;

      try {
        const job = await jobApi.getJobStatus({
          jobId: activeJobId,
          sessionId: activeSessionId,
        });

        if (isDisposed) {
          return;
        }

        const polledAt = new Date().toISOString();
        if (isTerminalJobStatus(job.status)) {
          dispatch({ type: "terminal-state-captured", sourceJobId: activeJobId, job, polledAt });
          shouldContinuePolling = false;
        } else {
          dispatch({ type: "monitor-tick-succeeded", sourceJobId: activeJobId, job, polledAt });
        }
      } catch (error) {
        if (isDisposed) {
          return;
        }

        dispatch({
          type: "monitor-tick-failed",
          sourceJobId: activeJobId,
          polledAt: new Date().toISOString(),
          error: normalizeJobStatusError(error),
        });
      } finally {
        isRequestInFlight = false;

        if (!isDisposed && shouldContinuePolling) {
          timeoutId = setTimeout(() => {
            void pollJobStatus();
          }, monitorIntervalMs);
        }
      }
    };

    void pollJobStatus();

    return () => {
      isDisposed = true;
      if (timeoutId !== null) {
        clearTimeout(timeoutId);
      }
    };
  }, [jobApi, monitorIntervalMs, state.jobId, state.phase, state.sessionId]);

  useEffect(() => {
    if (!state.sessionId || !state.jobId) {
      return;
    }

    if (!pokemonResultsRefreshStatuses.has(state.jobStatus)) {
      return;
    }

    const refreshKey = `${state.sessionId}:${state.jobId}:${state.jobStatus}`;
    if (lastJobPokemonFetchKeyRef.current === refreshKey) {
      return;
    }

    lastJobPokemonFetchKeyRef.current = refreshKey;
    void refreshPokemonResults({
      sessionId: state.sessionId,
    });
  }, [refreshPokemonResults, state.jobId, state.jobStatus, state.sessionId]);

  const handleFileSelect = useCallback((file) => {
    if (!file) {
      dispatch({ type: "select-file", file: null });
      return;
    }

    const validationError = validateSelectedUploadFile(file);
    if (validationError) {
      dispatch({ type: "select-file", file: null });
      dispatch({ type: "set-error", error: validationError });
      return;
    }

    dispatch({ type: "select-file", file });
  }, []);

  const submitUpload = useCallback(
    async (file) => {
      if (!state.sessionId) {
        dispatch({
          type: "set-error",
          error: {
            code: "INVALID_SESSION",
            message: "Your session expired. We can try again.",
            debugMessage: "Session id was missing when attempting upload.",
          },
        });
        return;
      }

      const validationError = validateSelectedUploadFile(file);
      if (validationError) {
        dispatch({ type: "set-error", error: validationError });
        return;
      }

      dispatch({ type: "set-uploading" });

      try {
        const result = await uploadApi.submitUpload({
          file,
          sessionId: state.sessionId,
        });

        dispatch({
          type: "set-success",
          uploadId: result.uploadId,
          jobId: result.jobId,
        });
      } catch (error) {
        dispatch({
          type: "set-error",
          error: normalizeUploadError(error),
        });
      }
    },
    [state.sessionId, uploadApi],
  );

  const handleSubmit = useCallback(
    async (event) => {
      event.preventDefault();
      await submitUpload(state.selectedFile);
    },
    [state.selectedFile, submitUpload],
  );

  const handleUploadRetry = useCallback(() => {
    void submitUpload(state.selectedFile);
  }, [state.selectedFile, submitUpload]);

  const handleJobRetry = useCallback(async () => {
    if (state.jobStatus !== "FAILED") {
      return;
    }

    if (!state.sessionId) {
      dispatch({
        type: "retry-failed",
        error: {
          code: "INVALID_SESSION",
          message: "Your session expired. We can try again.",
          debugMessage: "Session id was missing when attempting job retry.",
        },
      });
      return;
    }

    if (!state.jobId) {
      dispatch({
        type: "retry-failed",
        error: {
          code: "JOB_NOT_FOUND",
          message: "This job is no longer available. Upload again to create a new job.",
          debugMessage: "Job id was missing when attempting retry.",
        },
      });
      return;
    }

    dispatch({ type: "retry-started" });

    try {
      const retryResult = await jobApi.retryJob({
        jobId: state.jobId,
        sessionId: state.sessionId,
      });

      dispatch({
        type: "retry-succeeded",
        jobId: retryResult.jobId,
        uploadId: retryResult.uploadId,
      });
    } catch (error) {
      dispatch({
        type: "retry-failed",
        error: normalizeRetryError(error),
      });
    }
  }, [jobApi, state.jobId, state.jobStatus, state.sessionId]);

  const handleChooseAnotherFile = useCallback(() => {
    dispatch({ type: "select-file", file: null });
  }, []);

  const canSubmit = useMemo(
    () =>
      Boolean(state.sessionId) &&
      state.selectedFile !== null &&
      state.phase !== uploadFlowPhases.SESSION_LOADING &&
      state.phase !== uploadFlowPhases.UPLOADING,
    [state.phase, state.selectedFile, state.sessionId],
  );

  const canRetryUpload = useMemo(
    () =>
      state.phase === uploadFlowPhases.ERROR && Boolean(state.sessionId) && state.selectedFile !== null && !isLoading,
    [isLoading, state.phase, state.selectedFile, state.sessionId],
  );

  const canRetryFailedJob = useMemo(
    () =>
      state.phase === uploadFlowPhases.SUCCESS &&
      state.jobStatus === "FAILED" &&
      Boolean(state.sessionId) &&
      Boolean(state.jobId) &&
      !state.isRetrying &&
      !isLoading,
    [isLoading, state.isRetrying, state.jobId, state.jobStatus, state.phase, state.sessionId],
  );

  const canRetry = state.phase === uploadFlowPhases.ERROR ? canRetryUpload : canRetryFailedJob;
  const retryHandler = state.phase === uploadFlowPhases.ERROR ? handleUploadRetry : handleJobRetry;
  const handlePokemonResultsRetry = useCallback(() => {
    void refreshPokemonResults({
      sessionId: state.sessionId,
    });
  }, [refreshPokemonResults, state.sessionId]);

  const handleResolvePendingOption = useCallback(
    async (readingId, optionId) => {
      const normalizedSessionID = typeof state.sessionId === "string" ? state.sessionId.trim() : "";
      if (!normalizedSessionID) {
        setPendingResolveError({
          code: "INVALID_SESSION",
          message: "Your session expired. We can try again.",
          debugMessage: "Session id was missing when attempting pending species resolve.",
        });
        return;
      }

      const normalizedReadingID = typeof readingId === "string" ? readingId.trim() : "";
      const normalizedOptionID = typeof optionId === "string" ? optionId.trim() : "";
      if (!normalizedReadingID || !normalizedOptionID) {
        setPendingResolveError({
          code: "INVALID_REQUEST",
          message: "Could not resolve pending species because the request was incomplete.",
          debugMessage: "Missing readingId or optionId.",
        });
        return;
      }

      setPendingResolveError(null);
      setResolvingReadingIds((current) =>
        current.includes(normalizedReadingID) ? current : [...current, normalizedReadingID],
      );

      try {
        await pokemonResultsApi.resolvePendingSpeciesReading({
          sessionId: normalizedSessionID,
          readingId: normalizedReadingID,
          optionId: normalizedOptionID,
        });
        await refreshPokemonResults({
          sessionId: normalizedSessionID,
        });
      } catch (error) {
        setPendingResolveError(normalizePokemonResultsError(error));
      } finally {
        setResolvingReadingIds((current) => current.filter((id) => id !== normalizedReadingID));
      }
    },
    [pokemonResultsApi, refreshPokemonResults, state.sessionId],
  );

  const handleDismissPendingReading = useCallback(
    async (readingId) => {
      const normalizedSessionID = typeof state.sessionId === "string" ? state.sessionId.trim() : "";
      if (!normalizedSessionID) {
        setPendingResolveError({
          code: "INVALID_SESSION",
          message: "Your session expired. We can try again.",
          debugMessage: "Session id was missing when attempting pending species dismiss.",
        });
        return;
      }

      const normalizedReadingID = typeof readingId === "string" ? readingId.trim() : "";
      if (!normalizedReadingID) {
        setPendingResolveError({
          code: "INVALID_REQUEST",
          message: "Could not dismiss this pending species because the request was incomplete.",
          debugMessage: "Missing readingId.",
        });
        return;
      }

      setPendingResolveError(null);
      setResolvingReadingIds((current) =>
        current.includes(normalizedReadingID) ? current : [...current, normalizedReadingID],
      );

      try {
        await pokemonResultsApi.dismissPendingSpeciesReading({
          sessionId: normalizedSessionID,
          readingId: normalizedReadingID,
        });
        await refreshPokemonResults({
          sessionId: normalizedSessionID,
        });
      } catch (error) {
        setPendingResolveError(normalizePokemonResultsError(error));
      } finally {
        setResolvingReadingIds((current) => current.filter((id) => id !== normalizedReadingID));
      }
    },
    [pokemonResultsApi, refreshPokemonResults, state.sessionId],
  );

  const handleRequestDeleteResult = useCallback((result) => {
    const normalizedResultID = typeof result?.id === "string" ? result.id.trim() : "";
    const speciesName =
      typeof result?.speciesName === "string" && result.speciesName.trim().length > 0
        ? result.speciesName.trim()
        : "this result";

    if (!normalizedResultID) {
      setPendingDeleteError({
        code: "INVALID_REQUEST",
        message: "Could not delete this result because its ID is missing.",
        debugMessage: "Missing result id when opening delete confirmation.",
      });
      return;
    }

    setPendingDeleteError(null);
    setDeleteModalState({
      isOpen: true,
      resultId: normalizedResultID,
      speciesName,
    });
  }, []);

  const handleCancelDeleteResult = useCallback(() => {
    setPendingDeleteError(null);
    setDeleteModalState((current) => {
      if (deletingResultIds.includes(current.resultId)) {
        return current;
      }

      return {
        isOpen: false,
        resultId: "",
        speciesName: "",
      };
    });
  }, [deletingResultIds]);

  const handleConfirmDeleteResult = useCallback(async () => {
    const normalizedSessionID = typeof state.sessionId === "string" ? state.sessionId.trim() : "";
    if (!normalizedSessionID) {
      setPendingDeleteError({
        code: "INVALID_SESSION",
        message: "Your session expired. We can try again.",
        debugMessage: "Session id was missing when attempting result delete.",
      });
      return;
    }

    const normalizedResultID = deleteModalState.resultId.trim();
    if (!normalizedResultID) {
      setPendingDeleteError({
        code: "INVALID_REQUEST",
        message: "Could not delete this result because the request was incomplete.",
        debugMessage: "Missing resultId in delete modal state.",
      });
      return;
    }

    setPendingDeleteError(null);
    setDeletingResultIds((current) => (current.includes(normalizedResultID) ? current : [...current, normalizedResultID]));

    try {
      await pokemonResultsApi.deletePokemonResult({
        sessionId: normalizedSessionID,
        resultId: normalizedResultID,
      });
      setDeleteModalState({
        isOpen: false,
        resultId: "",
        speciesName: "",
      });
      await refreshPokemonResults({
        sessionId: normalizedSessionID,
        preserveItems: false,
      });
    } catch (error) {
      setPendingDeleteError(normalizePokemonResultsError(error));
    } finally {
      setDeletingResultIds((current) => current.filter((id) => id !== normalizedResultID));
    }
  }, [deleteModalState.resultId, pokemonResultsApi, refreshPokemonResults, state.sessionId]);

  return (
    <main className="min-h-screen bg-gradient-to-b from-slate-950 via-slate-900 to-slate-950">
      <div className="mx-auto flex w-full max-w-md flex-col gap-6 px-4 pb-10 pt-8 sm:max-w-xl sm:px-6">
        <header className="space-y-2">
          <h1 className="text-3xl font-semibold leading-tight text-white">Pokemon GO Appraisal Upload</h1>
          <p className="text-sm text-slate-300">
            Upload a screenshot or short video to start processing your appraisal.
          </p>
        </header>

        {identity.mode === "guest" ? (
          <section className="rounded-2xl border border-amber-400/30 bg-amber-500/10 p-4 text-amber-50 shadow-xl shadow-slate-950/40 sm:p-5">
            <p className="text-xs font-semibold uppercase tracking-[0.24em]">Guest Session</p>
            <p className="mt-2 text-sm leading-6">
              Guest scans can disappear at any time and will be cleared when you sign in. Sign up to keep your
              records saved and synced across devices.
            </p>
          </section>
        ) : null}

        <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4 shadow-xl shadow-slate-950/60 sm:p-6">
          <div className="mb-4 flex items-center justify-between gap-3">
            <span className="rounded-full border border-cyan-400/40 bg-cyan-400/10 px-3 py-1 text-xs font-medium text-cyan-200">
              {phaseLabel(state.phase)}
            </span>
            {isDebugMode ? (
              <span className="text-xs text-slate-400">
                {state.sessionId ? `Session ${state.sessionId.slice(0, 8)}...` : "Anonymous session"}
              </span>
            ) : null}
          </div>

          <UploadForm
            canSubmit={canSubmit}
            isSessionLoading={state.phase === uploadFlowPhases.SESSION_LOADING}
            isSubmitting={state.phase === uploadFlowPhases.UPLOADING}
            onSelectFile={handleFileSelect}
            onSubmit={handleSubmit}
            selectedFile={state.selectedFile}
          />

          <UploadStatusPanel
            canRetry={canRetry}
            error={state.error}
            finishedAt={state.finishedAt}
            identityMode={identity.mode}
            isRetrying={state.isRetrying}
            jobError={state.jobError}
            jobId={state.jobId}
            jobProgress={state.jobProgress}
            jobStage={state.jobStage}
            jobStatus={state.jobStatus}
            lastPolledAt={state.lastPolledAt}
            onChooseAnotherFile={handleChooseAnotherFile}
            onRetry={retryHandler}
            phase={state.phase}
            uploadId={state.uploadId}
            isDebugMode={isDebugMode}
          />

          <div className="mt-5 rounded-xl border border-slate-800 bg-slate-950/60 p-3 text-sm text-slate-300">
            <p className="font-medium text-slate-100">Upload summary</p>
            <p className="mt-1 break-all">Status: {phaseLabel(state.phase)}</p>
            <p className="mt-1 break-all">
              Selected file: {state.selectedFile ? state.selectedFile.name : "No file selected"}
            </p>
            {isDebugMode && state.uploadId ? <p className="mt-1 break-all">Upload ID: {state.uploadId}</p> : null}
            {isDebugMode && state.jobId ? <p className="mt-1 break-all">Job ID: {state.jobId}</p> : null}
            {state.isRetrying ? <p className="mt-1 break-all text-amber-200">Retrying failed job...</p> : null}
            {state.jobStatus ? <p className="mt-1 break-all">Job status: {state.jobStatus}</p> : null}
            {state.jobStage ? <p className="mt-1 break-all">Job stage: {state.jobStage}</p> : null}
            {state.jobStatus ? <p className="mt-1 break-all">Job progress: {state.jobProgress}%</p> : null}
            {state.finishedAt ? <p className="mt-1 break-all">Job finished at: {state.finishedAt}</p> : null}
            {state.lastPolledAt ? <p className="mt-1 break-all">Last polled at: {state.lastPolledAt}</p> : null}
            {state.jobError ? (
              <p className="mt-1 break-all text-rose-300">
                Job error ({state.jobError.code || "UNKNOWN"}): {state.jobError.message || "Processing failed."}
              </p>
            ) : null}
            {state.error ? (
              <p className="mt-1 break-all text-rose-300">
                Error ({state.error.code}): {state.error.message}
              </p>
            ) : null}
            {state.error && state.error.debugMessage ? (
              <p className="mt-1 break-all text-xs text-rose-200/80">Debug: {state.error.debugMessage}</p>
            ) : null}
          </div>

          <PokemonResultsPanel
            deleteConfirmation={deleteModalState.isOpen ? deleteModalState : null}
            deletingResultIds={deletingResultIds}
            error={pokemonResultsState.error}
            lastFetchedAt={pokemonResultsState.lastFetchedAt}
            onCancelDeleteResult={handleCancelDeleteResult}
            onConfirmDeleteResult={handleConfirmDeleteResult}
            onDismissPendingReading={handleDismissPendingReading}
            onRequestDeleteResult={handleRequestDeleteResult}
            onRetry={handlePokemonResultsRetry}
            onResolvePendingOption={handleResolvePendingOption}
            pendingDeleteError={pendingDeleteError}
            pendingReadings={pokemonResultsState.pendingItems}
            pendingResolveError={pendingResolveError}
            phase={pokemonResultsState.phase}
            resolvingReadingIds={resolvingReadingIds}
            results={pokemonResultsState.items}
            isDebugMode={isDebugMode}
          />

          <div className="mt-4 flex justify-end">
            <label className="inline-flex items-center gap-2 text-xs text-slate-400" htmlFor="debug-mode-checkbox">
              <input
                checked={isDebugMode}
                className="h-3.5 w-3.5 rounded border-slate-600 bg-slate-900 text-cyan-400 focus:ring-cyan-400/60"
                id="debug-mode-checkbox"
                onChange={(event) => {
                  setIsDebugMode(event.target.checked);
                }}
                type="checkbox"
              />
              Debug mode
            </label>
          </div>
        </section>
      </div>
    </main>
  );
}
