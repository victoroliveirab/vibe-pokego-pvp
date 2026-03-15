import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import UploadPage from "./UploadPage";

function createSessionHook() {
  return () => ({
    sessionId: "session-1",
    isLoading: false,
    error: null,
  });
}

function createPokemonResultsApi({ getPokemonResults } = {}) {
  return {
    getPokemonResults: getPokemonResults || vi.fn().mockResolvedValue({ results: [] }),
    getPendingSpeciesReadings: vi.fn().mockResolvedValue({ readings: [] }),
    resolvePendingSpeciesReading: vi.fn().mockResolvedValue({
      result: createPokemonResult(),
    }),
  };
}

function createDeferred() {
  let resolve;
  let reject;

  const promise = new Promise((promiseResolve, promiseReject) => {
    resolve = promiseResolve;
    reject = promiseReject;
  });

  return {
    promise,
    resolve,
    reject,
  };
}

function uploadInput() {
  return screen.getByLabelText("Choose image or video");
}

function createJobStatus({
  jobId,
  status,
  progress,
  stage,
  finishedAt = null,
  error = null,
}) {
  return {
    jobId,
    uploadId: "upload-1",
    status,
    progress,
    stage,
    createdAt: "2026-03-05T20:10:00Z",
    updatedAt: "2026-03-05T20:10:15Z",
    finishedAt,
    error,
  };
}

function createPokemonResult(overrides = {}) {
  return {
    id: "result-1",
    speciesName: "Machop",
    cp: 512,
    hp: 64,
    powerUpStardustCost: 2500,
    ivs: {
      attack: 12,
      defense: 15,
      stamina: 13,
    },
    level: {
      estimate: 23.5,
      confidence: 0.72,
      method: "ARC_POSITION",
    },
    source: {
      type: "VIDEO",
      uploadId: "upload-1",
      jobId: "job-1",
      timeRangeMs: {
        start: 12000,
        end: 15500,
      },
      frameTimestampMs: 13200,
    },
    confidence: 0.86,
    createdAt: "2026-03-05T20:10:15Z",
    ...overrides,
  };
}

describe("upload page job monitoring", () => {
  it("polls job status until terminal state and then stops", async () => {
    const uploadApi = {
      submitUpload: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    const jobApi = {
      getJobStatus: vi
        .fn()
        .mockResolvedValueOnce(
          createJobStatus({
            jobId: "job-1",
            status: "PROCESSING",
            progress: 35,
            stage: "SAMPLING_FRAMES",
          }),
        )
        .mockResolvedValueOnce(
          createJobStatus({
            jobId: "job-1",
            status: "SUCCEEDED",
            progress: 100,
            stage: null,
            finishedAt: "2026-03-05T20:10:45Z",
          }),
        ),
    };

    const pokemonResultsApi = createPokemonResultsApi();

    render(
      <UploadPage
        jobApi={jobApi}
        monitorIntervalMs={100}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={uploadApi}
        useSessionHook={createSessionHook()}
      />,
    );

    const file = new File(["image"], "screenshot.png", { type: "image/png" });
    fireEvent.change(uploadInput(), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "Submit Upload" }));

    await waitFor(() => {
      expect(uploadApi.submitUpload).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(jobApi.getJobStatus).toHaveBeenCalledTimes(2);
    });

    await waitFor(() => {
      expect(screen.getByText("Processing complete")).toBeTruthy();
    });

    await new Promise((resolve) => {
      setTimeout(resolve, 250);
    });

    expect(jobApi.getJobStatus).toHaveBeenCalledTimes(2);
  });

  it("surfaces transient polling errors and keeps monitoring", async () => {
    const uploadApi = {
      submitUpload: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    const jobApi = {
      getJobStatus: vi
        .fn()
        .mockRejectedValueOnce({
          code: "INTERNAL_ERROR",
          message: "Internal server error",
        })
        .mockResolvedValueOnce(
          createJobStatus({
            jobId: "job-1",
            status: "PROCESSING",
            progress: 62,
            stage: "EXTRACTING_APPRAISAL",
          }),
        ),
    };

    const pokemonResultsApi = createPokemonResultsApi();

    render(
      <UploadPage
        jobApi={jobApi}
        monitorIntervalMs={100}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={uploadApi}
        useSessionHook={createSessionHook()}
      />,
    );

    const file = new File(["image"], "screenshot.png", { type: "image/png" });
    fireEvent.change(uploadInput(), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "Submit Upload" }));

    await waitFor(() => {
      expect(jobApi.getJobStatus).toHaveBeenCalledTimes(1);
    });

    await waitFor(() => {
      expect(screen.getByText(/Status check issue:/)).toBeTruthy();
    });

    await waitFor(() => {
      expect(jobApi.getJobStatus).toHaveBeenCalledTimes(2);
    });

    await waitFor(() => {
      expect(screen.queryByText(/Status check issue:/)).toBeNull();
    });
  });

  it("retries failed jobs, swaps to the new job id, and resumes monitoring", async () => {
    const uploadApi = {
      submitUpload: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    let retriedJobPolls = 0;
    const jobApi = {
      getJobStatus: vi.fn().mockImplementation(async ({ jobId }) => {
        if (jobId === "job-1") {
          return createJobStatus({
            jobId: "job-1",
            status: "FAILED",
            progress: 100,
            stage: "POSTPROCESSING",
            finishedAt: "2026-03-05T20:10:45Z",
            error: {
              code: "NO_APPRAISALS_FOUND",
              message: "No readable appraisals detected",
            },
          });
        }

        retriedJobPolls += 1;
        if (retriedJobPolls === 1) {
          return createJobStatus({
            jobId: "job-2",
            status: "PROCESSING",
            progress: 35,
            stage: "SAMPLING_FRAMES",
          });
        }

        return createJobStatus({
          jobId: "job-2",
          status: "SUCCEEDED",
          progress: 100,
          stage: null,
          finishedAt: "2026-03-05T20:12:00Z",
        });
      }),
      retryJob: vi.fn().mockResolvedValue({
        jobId: "job-2",
        parentJobId: "job-1",
        uploadId: "upload-1",
        status: "QUEUED",
      }),
    };

    const pokemonResultsApi = createPokemonResultsApi();

    render(
      <UploadPage
        jobApi={jobApi}
        monitorIntervalMs={80}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={uploadApi}
        useSessionHook={createSessionHook()}
      />,
    );

    const file = new File(["image"], "screenshot.png", { type: "image/png" });
    fireEvent.change(uploadInput(), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "Submit Upload" }));

    await waitFor(() => {
      expect(screen.getByText("Processing failed")).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "Retry processing" }));

    await waitFor(() => {
      expect(jobApi.retryJob).toHaveBeenCalledWith({
        jobId: "job-1",
        sessionId: "session-1",
      });
    });

    await waitFor(() => {
      expect(screen.getByText("Processing complete")).toBeTruthy();
    });

    const polledJobIds = jobApi.getJobStatus.mock.calls.map(([args]) => args.jobId);
    expect(polledJobIds.filter((id) => id === "job-1")).toHaveLength(1);
    expect(polledJobIds.filter((id) => id === "job-2").length).toBeGreaterThan(0);
  });

  it("shows actionable retry-denied feedback and keeps failed job context", async () => {
    const uploadApi = {
      submitUpload: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    const jobApi = {
      getJobStatus: vi.fn().mockResolvedValue(
        createJobStatus({
          jobId: "job-1",
          status: "FAILED",
          progress: 100,
          stage: "POSTPROCESSING",
          finishedAt: "2026-03-05T20:10:45Z",
          error: {
            code: "NO_APPRAISALS_FOUND",
            message: "No readable appraisals detected",
          },
        }),
      ),
      retryJob: vi.fn().mockRejectedValue({
        code: "JOB_RETRY_NOT_ALLOWED",
        message: "Only failed jobs can be retried",
      }),
    };

    const pokemonResultsApi = createPokemonResultsApi();

    render(
      <UploadPage
        jobApi={jobApi}
        monitorIntervalMs={100}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={uploadApi}
        useSessionHook={createSessionHook()}
      />,
    );

    const file = new File(["image"], "screenshot.png", { type: "image/png" });
    fireEvent.change(uploadInput(), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "Submit Upload" }));

    await waitFor(() => {
      expect(screen.getByText("Processing failed")).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "Retry processing" }));

    await waitFor(() => {
      expect(jobApi.retryJob).toHaveBeenCalledWith({
        jobId: "job-1",
        sessionId: "session-1",
      });
    });

    await waitFor(() => {
      expect(screen.getByText("Retry failed: Only failed jobs can be retried")).toBeTruthy();
    });

    expect(screen.getByText("Retry code: JOB_RETRY_NOT_ALLOWED")).toBeTruthy();
    expect(screen.queryByText("Job ID: job-1")).toBeNull();
  });

  it("keeps IDs hidden by default and shows them when debug mode is enabled", async () => {
    const uploadApi = {
      submitUpload: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    const pokemonResultsApi = createPokemonResultsApi();

    render(
      <UploadPage
        jobApi={{ getJobStatus: vi.fn() }}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={uploadApi}
        useSessionHook={createSessionHook()}
      />,
    );

    const debugCheckbox = screen.getByLabelText("Debug mode");
    expect(debugCheckbox.checked).toBe(false);
    expect(screen.queryByText("Session session-...")).toBeNull();

    const file = new File(["image"], "screenshot.png", { type: "image/png" });
    fireEvent.change(uploadInput(), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "Submit Upload" }));

    await waitFor(() => {
      expect(uploadApi.submitUpload).toHaveBeenCalledTimes(1);
    });

    expect(screen.queryByText("Upload ID: upload-1")).toBeNull();
    expect(screen.queryByText("Job ID: job-1")).toBeNull();

    fireEvent.click(debugCheckbox);

    expect(debugCheckbox.checked).toBe(true);
    expect(screen.getByText("Session session-...")).toBeTruthy();
    expect(screen.getByText("Upload ID: upload-1")).toBeTruthy();
    expect(screen.getByText("Job ID: job-1")).toBeTruthy();
  });

  it("fetches pokemon results when the session is ready", async () => {
    const pokemonResultsApi = createPokemonResultsApi();

    render(
      <UploadPage
        jobApi={{ getJobStatus: vi.fn() }}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={{ submitUpload: vi.fn() }}
        useSessionHook={createSessionHook()}
      />,
    );

    await waitFor(() => {
      expect(pokemonResultsApi.getPokemonResults).toHaveBeenCalledTimes(1);
    });

    expect(pokemonResultsApi.getPokemonResults).toHaveBeenCalledWith({
      sessionId: "session-1",
    });
  });

  it("refreshes pokemon results when a job reaches success", async () => {
    const uploadApi = {
      submitUpload: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    const jobApi = {
      getJobStatus: vi
        .fn()
        .mockResolvedValueOnce(
          createJobStatus({
            jobId: "job-1",
            status: "PROCESSING",
            progress: 45,
            stage: "EXTRACTING_APPRAISAL",
          }),
        )
        .mockResolvedValueOnce(
          createJobStatus({
            jobId: "job-1",
            status: "SUCCEEDED",
            progress: 100,
            stage: null,
            finishedAt: "2026-03-05T20:15:00Z",
          }),
        ),
    };

    const pokemonResultsApi = createPokemonResultsApi();

    render(
      <UploadPage
        jobApi={jobApi}
        monitorIntervalMs={50}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={uploadApi}
        useSessionHook={createSessionHook()}
      />,
    );

    const file = new File(["image"], "screenshot.png", { type: "image/png" });
    fireEvent.change(uploadInput(), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "Submit Upload" }));

    await waitFor(() => {
      expect(jobApi.getJobStatus).toHaveBeenCalledTimes(2);
    });

    await waitFor(() => {
      expect(pokemonResultsApi.getPokemonResults).toHaveBeenCalledTimes(2);
    });
  });

  it("ignores stale pokemon results responses from earlier requests", async () => {
    const uploadApi = {
      submitUpload: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    const jobApi = {
      getJobStatus: vi
        .fn()
        .mockResolvedValueOnce(
          createJobStatus({
            jobId: "job-1",
            status: "PROCESSING",
            progress: 55,
            stage: "EXTRACTING_APPRAISAL",
          }),
        )
        .mockResolvedValueOnce(
          createJobStatus({
            jobId: "job-1",
            status: "SUCCEEDED",
            progress: 100,
            stage: null,
            finishedAt: "2026-03-05T20:20:00Z",
          }),
        ),
    };

    const firstRequest = createDeferred();
    const pokemonResultsApi = createPokemonResultsApi({
      getPokemonResults: vi
        .fn()
        .mockReturnValueOnce(firstRequest.promise)
        .mockResolvedValueOnce({
          results: [
            createPokemonResult({ id: "fresh-1", speciesName: "Pikachu" }),
            createPokemonResult({ id: "fresh-2", speciesName: "Bulbasaur" }),
          ],
        }),
    });

    render(
      <UploadPage
        jobApi={jobApi}
        monitorIntervalMs={50}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={uploadApi}
        useSessionHook={createSessionHook()}
      />,
    );

    await waitFor(() => {
      expect(pokemonResultsApi.getPokemonResults).toHaveBeenCalledTimes(1);
    });

    const file = new File(["image"], "screenshot.png", { type: "image/png" });
    fireEvent.change(uploadInput(), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "Submit Upload" }));

    await waitFor(() => {
      expect(jobApi.getJobStatus).toHaveBeenCalledTimes(2);
    });

    await waitFor(() => {
      expect(pokemonResultsApi.getPokemonResults).toHaveBeenCalledTimes(2);
    });

    await waitFor(() => {
      expect(screen.getAllByText("Pikachu").length).toBeGreaterThan(0);
      expect(screen.getAllByText("Bulbasaur").length).toBeGreaterThan(0);
    });

    firstRequest.resolve({
      results: [createPokemonResult({ id: "stale-1", speciesName: "Stale Species" })],
    });

    await waitFor(() => {
      expect(screen.getAllByText("Pikachu").length).toBeGreaterThan(0);
      expect(screen.queryByText("Stale Species")).toBeNull();
    });
  });

  it("refreshes pokemon results after failed job retry succeeds", async () => {
    const uploadApi = {
      submitUpload: vi.fn().mockResolvedValue({
        uploadId: "upload-1",
        jobId: "job-1",
      }),
    };

    let retriedJobPolls = 0;
    const jobApi = {
      getJobStatus: vi.fn().mockImplementation(async ({ jobId }) => {
        if (jobId === "job-1") {
          return createJobStatus({
            jobId: "job-1",
            status: "FAILED",
            progress: 100,
            stage: "POSTPROCESSING",
            finishedAt: "2026-03-05T20:30:00Z",
            error: {
              code: "NO_APPRAISALS_FOUND",
              message: "No readable appraisals detected",
            },
          });
        }

        retriedJobPolls += 1;
        if (retriedJobPolls === 1) {
          return createJobStatus({
            jobId: "job-2",
            status: "PROCESSING",
            progress: 48,
            stage: "SAMPLING_FRAMES",
          });
        }

        return createJobStatus({
          jobId: "job-2",
          status: "SUCCEEDED",
          progress: 100,
          stage: null,
          finishedAt: "2026-03-05T20:31:00Z",
        });
      }),
      retryJob: vi.fn().mockResolvedValue({
        jobId: "job-2",
        parentJobId: "job-1",
        uploadId: "upload-1",
        status: "QUEUED",
      }),
    };

    const pokemonResultsApi = createPokemonResultsApi({
      getPokemonResults: vi
        .fn()
        .mockResolvedValueOnce({ results: [createPokemonResult({ id: "before-retry", speciesName: "Machop" })] })
        .mockResolvedValueOnce({
          results: [createPokemonResult({ id: "after-retry", speciesName: "Dragonite", cp: 3141 })],
        }),
    });

    render(
      <UploadPage
        jobApi={jobApi}
        monitorIntervalMs={50}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={uploadApi}
        useSessionHook={createSessionHook()}
      />,
    );

    await waitFor(() => {
      expect(screen.getAllByText("Machop").length).toBeGreaterThan(0);
    });

    const file = new File(["image"], "screenshot.png", { type: "image/png" });
    fireEvent.change(uploadInput(), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "Submit Upload" }));

    await waitFor(() => {
      expect(screen.getByText("Processing failed")).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "Retry processing" }));

    await waitFor(() => {
      expect(screen.getByText("Processing complete")).toBeTruthy();
    });

    await waitFor(() => {
      expect(pokemonResultsApi.getPokemonResults).toHaveBeenCalledTimes(2);
    });

    await waitFor(() => {
      expect(screen.getAllByText("Dragonite").length).toBeGreaterThan(0);
      expect(screen.queryByText("Machop")).toBeNull();
    });
  });

  it("resolves pending species reading and refreshes results", async () => {
    const pendingReading = {
      id: "reading-1",
      jobId: "job-1",
      uploadId: "upload-1",
      cp: 712,
      hp: 120,
      ivs: {
        attack: 10,
        defense: 11,
        stamina: 12,
      },
      level: {
        estimate: 23.5,
        confidence: 0.72,
        method: "ARC_POSITION",
      },
      source: {
        type: "VIDEO",
        frameTimestampMs: 300,
      },
      confidence: 0.86,
      status: "PENDING_USER_DEDUP",
      createdAt: "2026-03-06T17:00:00Z",
      options: [
        {
          id: "option-1",
          speciesName: "Darumaka",
          matchMode: "exact",
          matchDistance: 0,
          optionRank: 1,
        },
      ],
    };

    const pokemonResultsApi = createPokemonResultsApi({
      getPokemonResults: vi
        .fn()
        .mockResolvedValueOnce({ results: [] })
        .mockResolvedValueOnce({ results: [createPokemonResult({ speciesName: "Darumaka" })] }),
    });
    pokemonResultsApi.getPendingSpeciesReadings = vi
      .fn()
      .mockResolvedValueOnce({ readings: [pendingReading] })
      .mockResolvedValueOnce({ readings: [] });
    pokemonResultsApi.resolvePendingSpeciesReading = vi.fn().mockResolvedValue({
      result: createPokemonResult({ speciesName: "Darumaka" }),
    });

    render(
      <UploadPage
        jobApi={{ getJobStatus: vi.fn() }}
        pokemonResultsApi={pokemonResultsApi}
        uploadApi={{ submitUpload: vi.fn() }}
        useSessionHook={createSessionHook()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Pending Species Confirmation")).toBeTruthy();
      expect(screen.getByRole("button", { name: /Darumaka/ })).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: /Darumaka/ }));

    await waitFor(() => {
      expect(pokemonResultsApi.resolvePendingSpeciesReading).toHaveBeenCalledWith({
        sessionId: "session-1",
        readingId: "reading-1",
        optionId: "option-1",
      });
    });

    await waitFor(() => {
      expect(screen.queryByText("Pending Species Confirmation")).toBeNull();
      expect(screen.getAllByText("Darumaka").length).toBeGreaterThan(0);
    });
  });
});
