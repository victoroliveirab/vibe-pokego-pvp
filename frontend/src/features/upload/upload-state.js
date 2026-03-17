export const uploadFlowPhases = {
  IDLE: "idle",
  SESSION_LOADING: "session-loading",
  READY: "ready",
  UPLOADING: "uploading",
  SUCCESS: "success",
  ERROR: "error",
};

export const initialJobLifecycleState = {
  jobStatus: "",
  jobStage: "",
  jobProgress: 0,
  jobError: null,
  finishedAt: "",
  lastPolledAt: "",
};

function createResetJobLifecycleState(overrides = {}) {
  return {
    ...initialJobLifecycleState,
    ...overrides,
  };
}

function normalizeJobLifecycleError(error) {
  if (!error || typeof error !== "object") {
    return null;
  }

  return {
    code: typeof error.code === "string" ? error.code : "",
    message: typeof error.message === "string" ? error.message : "",
    debugMessage: typeof error.debugMessage === "string" ? error.debugMessage : "",
    details: Object.hasOwn(error, "details") ? error.details : null,
  };
}

function applyJobSnapshot(state, action) {
  const sourceJobId = typeof action.sourceJobId === "string" ? action.sourceJobId : "";
  if (sourceJobId && state.jobId && sourceJobId !== state.jobId) {
    return state;
  }

  const job = action.job && typeof action.job === "object" ? action.job : {};

  return {
    ...state,
    jobId: typeof job.jobId === "string" && job.jobId.trim().length > 0 ? job.jobId : state.jobId,
    jobStatus: typeof job.status === "string" ? job.status : "",
    jobStage: typeof job.stage === "string" ? job.stage : "",
    jobProgress: typeof job.progress === "number" && !Number.isNaN(job.progress) ? job.progress : 0,
    jobError: normalizeJobLifecycleError(job.error),
    finishedAt: typeof job.finishedAt === "string" ? job.finishedAt : "",
    lastPolledAt: typeof action.polledAt === "string" ? action.polledAt : state.lastPolledAt,
  };
}

export const initialUploadState = {
  phase: uploadFlowPhases.IDLE,
  sessionId: "",
  selectedFile: null,
  uploadId: "",
  jobId: "",
  isRetrying: false,
  error: null,
  ...initialJobLifecycleState,
};

export function uploadStateReducer(state, action) {
  switch (action.type) {
    case "select-file":
      return {
        ...state,
        selectedFile: action.file ?? null,
        uploadId: "",
        jobId: "",
        isRetrying: false,
        error: null,
        ...createResetJobLifecycleState(),
        phase: state.sessionId ? uploadFlowPhases.READY : state.phase,
      };
    case "set-uploading":
      return {
        ...state,
        phase: uploadFlowPhases.UPLOADING,
        uploadId: "",
        jobId: "",
        isRetrying: false,
        error: null,
        ...createResetJobLifecycleState(),
      };
    case "set-phase":
      return {
        ...state,
        phase: action.phase,
      };
    case "set-session":
      return {
        ...state,
        sessionId: action.sessionId,
        isRetrying: false,
        error: null,
        phase: uploadFlowPhases.READY,
      };
    case "set-success":
      return {
        ...state,
        phase: uploadFlowPhases.SUCCESS,
        uploadId: action.uploadId,
        jobId: action.jobId,
        isRetrying: false,
        error: null,
        ...createResetJobLifecycleState({
          jobStatus: "QUEUED",
        }),
      };
    case "resume-active-job":
      return applyJobSnapshot(
        {
          ...state,
          phase: uploadFlowPhases.SUCCESS,
          uploadId: action.job && typeof action.job.uploadId === "string" ? action.job.uploadId : "",
          jobId: action.job && typeof action.job.jobId === "string" ? action.job.jobId : "",
          isRetrying: false,
          error: null,
          ...createResetJobLifecycleState(),
        },
        {
          ...action,
          sourceJobId: action.job && typeof action.job.jobId === "string" ? action.job.jobId : "",
        },
      );
    case "set-error":
      return {
        ...state,
        phase: uploadFlowPhases.ERROR,
        isRetrying: false,
        error: action.error,
      };
    case "monitor-tick-succeeded":
      return applyJobSnapshot(
        {
          ...state,
          error: null,
        },
        action,
      );
    case "monitor-tick-failed":
      if (
        typeof action.sourceJobId === "string" &&
        action.sourceJobId.trim().length > 0 &&
        state.jobId &&
        action.sourceJobId !== state.jobId
      ) {
        return state;
      }

      return {
        ...state,
        error: action.error,
        lastPolledAt: typeof action.polledAt === "string" ? action.polledAt : state.lastPolledAt,
      };
    case "terminal-state-captured":
      return applyJobSnapshot(
        {
          ...state,
          error: null,
        },
        action,
      );
    case "retry-started":
      return {
        ...state,
        isRetrying: true,
        error: null,
      };
    case "retry-succeeded":
      return {
        ...state,
        phase: uploadFlowPhases.SUCCESS,
        jobId: action.jobId,
        uploadId:
          typeof action.uploadId === "string" && action.uploadId.trim().length > 0 ? action.uploadId : state.uploadId,
        isRetrying: false,
        error: null,
        ...createResetJobLifecycleState({
          jobStatus: "QUEUED",
        }),
      };
    case "retry-failed":
      return {
        ...state,
        isRetrying: false,
        error: action.error,
      };
    default:
      return state;
  }
}
