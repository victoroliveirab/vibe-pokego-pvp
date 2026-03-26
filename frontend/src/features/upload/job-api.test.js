import { APIClientError } from "../../lib/api-errors";
import { createJobApi, normalizeJobStatusError, normalizeRetryError } from "./job-api";

describe("job api", () => {
  it("gets job status with session context and normalizes payload", async () => {
    const apiClient = {
      request: vi.fn().mockResolvedValue({
        jobId: "job-1",
        uploadId: "upload-1",
        status: "PROCESSING",
        progress: 41.7,
        stage: "SAMPLING_FRAMES",
        progressDescription: "sampling 73/90 frames",
        createdAt: "2026-03-05T12:00:00Z",
        updatedAt: "2026-03-05T12:00:20Z",
        finishedAt: null,
        error: null,
      }),
    };

    const jobApi = createJobApi({ apiClient });
    const result = await jobApi.getJobStatus({
      jobId: "job-1",
      sessionId: "session-1",
    });

    expect(result).toEqual({
      jobId: "job-1",
      uploadId: "upload-1",
      status: "PROCESSING",
      progress: 41.7,
      stage: "SAMPLING_FRAMES",
      progressDescription: "sampling 73/90 frames",
      createdAt: "2026-03-05T12:00:00Z",
      updatedAt: "2026-03-05T12:00:20Z",
      finishedAt: null,
      error: null,
    });

    expect(apiClient.request).toHaveBeenCalledWith(
      "/jobs/job-1",
      expect.objectContaining({
        method: "GET",
        requiresIdentity: true,
        sessionId: "session-1",
      }),
    );
  });

  it("throws INVALID_RESPONSE when required status payload fields are missing", async () => {
    const jobApi = createJobApi({
      apiClient: {
        request: vi.fn().mockResolvedValue({
          jobId: "job-1",
          uploadId: "upload-1",
          progress: 50,
          createdAt: "2026-03-05T12:00:00Z",
          updatedAt: "2026-03-05T12:00:20Z",
        }),
      },
    });

    await expect(
      jobApi.getJobStatus({
        jobId: "job-1",
        sessionId: "session-1",
      }),
    ).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
  });

  it("retries failed job and normalizes retry payload", async () => {
    const apiClient = {
      request: vi.fn().mockResolvedValue({
        jobId: "job-2",
        parentJobId: "job-1",
        uploadId: "upload-1",
        status: "QUEUED",
      }),
    };

    const jobApi = createJobApi({ apiClient });
    const result = await jobApi.retryJob({
      jobId: "job-1",
      sessionId: "session-1",
    });

    expect(result).toEqual({
      jobId: "job-2",
      parentJobId: "job-1",
      uploadId: "upload-1",
      status: "QUEUED",
    });

    expect(apiClient.request).toHaveBeenCalledWith(
      "/jobs/job-1/retry",
      expect.objectContaining({
        method: "POST",
        requiresIdentity: true,
        sessionId: "session-1",
      }),
    );
  });

  it("throws when retry response is missing expected identifiers", async () => {
    const jobApi = createJobApi({
      apiClient: {
        request: vi.fn().mockResolvedValue({
          jobId: "job-2",
          uploadId: "upload-1",
          status: "QUEUED",
        }),
      },
    });

    await expect(
      jobApi.retryJob({
        jobId: "job-1",
        sessionId: "session-1",
      }),
    ).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
  });

  it("normalizes job status errors with user-facing mapping", () => {
    const normalized = normalizeJobStatusError(
      new APIClientError({
        code: "JOB_NOT_FOUND",
        message: "Job not found",
        status: 404,
      }),
    );

    expect(normalized).toEqual({
      code: "JOB_NOT_FOUND",
      message: "This job is no longer available. Upload again to create a new job.",
      debugMessage: "Job not found",
      details: null,
    });
  });

  it("normalizes retry errors with retry-specific mapping", () => {
    const normalized = normalizeRetryError(
      new APIClientError({
        code: "JOB_RETRY_NOT_ALLOWED",
        message: "Only failed jobs can be retried",
        status: 409,
      }),
    );

    expect(normalized).toEqual({
      code: "JOB_RETRY_NOT_ALLOWED",
      message: "Only failed jobs can be retried.",
      debugMessage: "Only failed jobs can be retried",
      details: null,
    });
  });
});
