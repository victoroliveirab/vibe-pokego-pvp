import { fireEvent, render, screen } from "@testing-library/react";
import UploadStatusPanel from "./UploadStatusPanel";
import { uploadFlowPhases } from "./upload-state";

function createPanelProps(overrides = {}) {
  return {
    canRetry: false,
    error: null,
    finishedAt: "",
    isRetrying: false,
    jobError: null,
    jobId: "job-123",
    jobProgress: 0,
    jobStage: "",
    jobStatus: "QUEUED",
    lastPolledAt: "",
    onChooseAnotherFile: vi.fn(),
    onRetry: vi.fn(),
    phase: uploadFlowPhases.SUCCESS,
    uploadId: "upload-123",
    ...overrides,
  };
}

describe("upload status panel", () => {
  it("renders queued lifecycle status", () => {
    render(<UploadStatusPanel {...createPanelProps({ jobStatus: "QUEUED" })} />);

    expect(screen.getByText("Queued for processing")).toBeTruthy();
    expect(screen.getByText("Stage: Waiting for worker")).toBeTruthy();
    expect(screen.getByText("Progress: 0%")).toBeTruthy();
    expect(screen.getByText("upload-123")).toBeTruthy();
    expect(screen.getByText("job-123")).toBeTruthy();
  });

  it("renders processing lifecycle status with stage and progress bar", () => {
    render(
      <UploadStatusPanel
        {...createPanelProps({
          jobStatus: "PROCESSING",
          jobProgress: 42,
          jobStage: "SAMPLING_FRAMES",
        })}
      />,
    );

    expect(screen.getByText("Processing")).toBeTruthy();
    expect(screen.getByText("Stage: Sampling Frames")).toBeTruthy();
    expect(screen.getByText("Progress: 42%")).toBeTruthy();
    expect(screen.getByRole("progressbar", { name: "Job progress" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Retry processing" })).toBeNull();
  });

  it("renders succeeded terminal status with follow-up action", () => {
    const onChooseAnotherFile = vi.fn();

    render(
      <UploadStatusPanel
        {...createPanelProps({
          jobStatus: "SUCCEEDED",
          finishedAt: "2026-03-05T20:12:05Z",
          onChooseAnotherFile,
        })}
      />,
    );

    expect(screen.getByText("Processing complete")).toBeTruthy();
    expect(screen.getByText("Finished at: 2026-03-05T20:12:05Z")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Choose another file" }));
    expect(onChooseAnotherFile).toHaveBeenCalledTimes(1);
  });

  it("renders pending-user-dedup terminal state explanation", () => {
    render(<UploadStatusPanel {...createPanelProps({ jobStatus: "PENDING_USER_DEDUP" })} />);

    expect(screen.getByText("Waiting for your species selection")).toBeTruthy();
    expect(
      screen.getByText("Processing finished, but we need a species confirmation before finalizing results."),
    ).toBeTruthy();
  });

  it("renders failed lifecycle state with actionable context and retry", () => {
    const onRetry = vi.fn();

    render(
      <UploadStatusPanel
        {...createPanelProps({
          canRetry: true,
          jobStatus: "FAILED",
          jobProgress: 96,
          jobStage: "POSTPROCESSING",
          jobError: {
            code: "NO_APPRAISALS_FOUND",
            message: "No readable appraisals detected",
          },
          finishedAt: "2026-03-05T20:12:05Z",
          onRetry,
        })}
      />,
    );

    expect(screen.getByText("Processing failed")).toBeTruthy();
    expect(screen.getByText("No readable appraisals detected")).toBeTruthy();
    expect(screen.getByText("Code: NO_APPRAISALS_FOUND")).toBeTruthy();
    expect(screen.getByText("Stage: Postprocessing")).toBeTruthy();
    expect(screen.getByText("Progress: 96%")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry processing" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("renders retry failure context for failed jobs", () => {
    render(
      <UploadStatusPanel
        {...createPanelProps({
          canRetry: true,
          jobStatus: "FAILED",
          error: {
            code: "JOB_RETRY_NOT_ALLOWED",
            message: "Only failed jobs can be retried.",
            debugMessage: "Only failed jobs can be retried",
          },
        })}
      />,
    );

    expect(screen.getByText("Retry failed: Only failed jobs can be retried.")).toBeTruthy();
    expect(screen.getByText("Retry code: JOB_RETRY_NOT_ALLOWED")).toBeTruthy();
  });

  it("renders upload request error panel with retry upload action", () => {
    const onRetry = vi.fn();

    render(
      <UploadStatusPanel
        {...createPanelProps({
          canRetry: true,
          phase: uploadFlowPhases.ERROR,
          error: {
            code: "VIDEO_TOO_LONG",
            message: "Video must be 90 seconds or shorter.",
            debugMessage: "Video duration exceeds 90 seconds",
          },
        })}
        onRetry={onRetry}
      />,
    );

    expect(screen.getByText("Upload failed")).toBeTruthy();
    expect(screen.getByText("Code: VIDEO_TOO_LONG")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry upload" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
