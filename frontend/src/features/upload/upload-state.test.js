import { initialUploadState, uploadFlowPhases, uploadStateReducer } from "./upload-state";

describe("upload state reducer", () => {
  it("tracks initial job lifecycle fields", () => {
    expect(initialUploadState).toMatchObject({
      isRetrying: false,
      jobStatus: "",
      jobStage: "",
      jobProgress: 0,
      jobProgressDescription: "",
      jobError: null,
      finishedAt: "",
      lastPolledAt: "",
    });
  });

  it("sets upload success and seeds queued job lifecycle state", () => {
    const nextState = uploadStateReducer(
      {
        ...initialUploadState,
        sessionId: "session-1",
        phase: uploadFlowPhases.UPLOADING,
      },
      {
        type: "set-success",
        uploadId: "upload-1",
        jobId: "job-1",
      },
    );

    expect(nextState).toMatchObject({
      phase: uploadFlowPhases.SUCCESS,
      uploadId: "upload-1",
      jobId: "job-1",
      isRetrying: false,
      jobStatus: "QUEUED",
      jobStage: "",
      jobProgress: 0,
      jobProgressDescription: "",
      jobError: null,
      finishedAt: "",
      lastPolledAt: "",
      error: null,
    });
  });

  it("updates lifecycle fields on monitor tick success", () => {
    const nextState = uploadStateReducer(
      {
        ...initialUploadState,
        phase: uploadFlowPhases.SUCCESS,
        uploadId: "upload-1",
        jobId: "job-1",
        jobStatus: "QUEUED",
      },
      {
        type: "monitor-tick-succeeded",
        polledAt: "2026-03-05T20:10:00Z",
        job: {
          jobId: "job-1",
          status: "PROCESSING",
          progress: 52,
          stage: "DETECTING_APPRAISAL_SCREENS",
          progressDescription: "sampling 47/90 frames",
          finishedAt: null,
          error: null,
        },
      },
    );

    expect(nextState).toMatchObject({
      jobId: "job-1",
      jobStatus: "PROCESSING",
      jobStage: "DETECTING_APPRAISAL_SCREENS",
      jobProgress: 52,
      jobProgressDescription: "sampling 47/90 frames",
      jobError: null,
      finishedAt: "",
      lastPolledAt: "2026-03-05T20:10:00Z",
      error: null,
    });
  });

  it("captures terminal job state", () => {
    const nextState = uploadStateReducer(
      {
        ...initialUploadState,
        phase: uploadFlowPhases.SUCCESS,
        uploadId: "upload-1",
        jobId: "job-1",
        error: {
          code: "JOB_STATUS_FAILED",
          message: "Server error. Please try again.",
        },
      },
      {
        type: "terminal-state-captured",
        polledAt: "2026-03-05T20:11:00Z",
        job: {
          jobId: "job-1",
          status: "FAILED",
          progress: 100,
          stage: "POSTPROCESSING",
          progressDescription: null,
          finishedAt: "2026-03-05T20:10:58Z",
          error: {
            code: "NO_APPRAISALS_FOUND",
            message: "No readable appraisals detected",
          },
        },
      },
    );

    expect(nextState).toMatchObject({
      jobStatus: "FAILED",
      jobProgress: 100,
      jobStage: "POSTPROCESSING",
      jobProgressDescription: "",
      finishedAt: "2026-03-05T20:10:58Z",
      lastPolledAt: "2026-03-05T20:11:00Z",
      error: null,
      jobError: {
        code: "NO_APPRAISALS_FOUND",
        message: "No readable appraisals detected",
      },
    });
  });

  it("tracks retry transitions and swaps active job id", () => {
    const retryStarted = uploadStateReducer(
      {
        ...initialUploadState,
        phase: uploadFlowPhases.SUCCESS,
        jobId: "job-1",
        error: {
          code: "JOB_RETRY_FAILED",
          message: "Could not retry.",
        },
      },
      {
        type: "retry-started",
      },
    );

    expect(retryStarted.error).toBeNull();
    expect(retryStarted.isRetrying).toBe(true);

    const retrySucceeded = uploadStateReducer(retryStarted, {
      type: "retry-succeeded",
      jobId: "job-2",
      uploadId: "upload-1",
    });

    expect(retrySucceeded).toMatchObject({
      phase: uploadFlowPhases.SUCCESS,
      jobId: "job-2",
      uploadId: "upload-1",
      isRetrying: false,
      jobStatus: "QUEUED",
      jobStage: "",
      jobProgress: 0,
      jobProgressDescription: "",
      jobError: null,
      finishedAt: "",
      lastPolledAt: "",
      error: null,
    });
  });

  it("stores monitor and retry failures without dropping active job context", () => {
    const monitorFailed = uploadStateReducer(
      {
        ...initialUploadState,
        phase: uploadFlowPhases.SUCCESS,
        jobId: "job-1",
      },
      {
        type: "monitor-tick-failed",
        polledAt: "2026-03-05T20:12:00Z",
        error: {
          code: "INTERNAL_ERROR",
          message: "Server error. Please try again.",
          debugMessage: "Internal server error",
        },
      },
    );

    expect(monitorFailed).toMatchObject({
      jobId: "job-1",
      lastPolledAt: "2026-03-05T20:12:00Z",
      isRetrying: false,
      error: {
        code: "INTERNAL_ERROR",
      },
    });

    const retryFailed = uploadStateReducer(monitorFailed, {
      type: "retry-failed",
      error: {
        code: "JOB_RETRY_NOT_ALLOWED",
        message: "Only failed jobs can be retried.",
      },
    });

    expect(retryFailed).toMatchObject({
      jobId: "job-1",
      isRetrying: false,
      error: {
        code: "JOB_RETRY_NOT_ALLOWED",
      },
    });
  });

  it("ignores stale monitor updates from a previous job id", () => {
    const nextState = uploadStateReducer(
      {
        ...initialUploadState,
        phase: uploadFlowPhases.SUCCESS,
        uploadId: "upload-1",
        jobId: "job-2",
        jobStatus: "QUEUED",
      },
      {
        type: "monitor-tick-succeeded",
        sourceJobId: "job-1",
        polledAt: "2026-03-05T20:13:00Z",
        job: {
          jobId: "job-1",
          status: "FAILED",
          progress: 100,
          stage: "POSTPROCESSING",
          finishedAt: "2026-03-05T20:12:59Z",
          error: {
            code: "NO_APPRAISALS_FOUND",
            message: "No readable appraisals detected",
          },
        },
      },
    );

    expect(nextState).toMatchObject({
      jobId: "job-2",
      jobStatus: "QUEUED",
      lastPolledAt: "",
      finishedAt: "",
      jobError: null,
    });
  });
});
