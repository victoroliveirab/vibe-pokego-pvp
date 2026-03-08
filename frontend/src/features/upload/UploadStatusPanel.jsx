import { uploadFlowPhases } from "./upload-state";

function statusToneClasses(phase) {
  if (phase === uploadFlowPhases.SUCCESS) {
    return "border-emerald-500/40 bg-emerald-500/10 text-emerald-100";
  }

  if (phase === uploadFlowPhases.ERROR) {
    return "border-rose-500/40 bg-rose-500/10 text-rose-100";
  }

  return "border-cyan-400/30 bg-cyan-400/10 text-cyan-100";
}

function formatJobStage(stage) {
  if (typeof stage !== "string" || stage.trim().length === 0) {
    return "Waiting for worker";
  }

  return stage
    .toLowerCase()
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

function clampProgress(progress) {
  if (typeof progress !== "number" || Number.isNaN(progress)) {
    return 0;
  }

  return Math.max(0, Math.min(100, Math.round(progress)));
}

function JobIdentity({ jobId, uploadId }) {
  return (
    <dl className="mt-3 space-y-1 text-xs text-slate-100/90">
      <div>
        <dt className="font-medium">Upload ID</dt>
        <dd className="break-all">{uploadId}</dd>
      </div>
      <div>
        <dt className="font-medium">Job ID</dt>
        <dd className="break-all">{jobId}</dd>
      </div>
    </dl>
  );
}

export default function UploadStatusPanel({
  canRetry,
  error,
  finishedAt,
  isRetrying,
  jobError,
  jobId,
  jobProgress,
  jobStage,
  jobStatus,
  lastPolledAt,
  onChooseAnotherFile,
  onRetry,
  phase,
  uploadId,
}) {
  if (phase === uploadFlowPhases.IDLE || phase === uploadFlowPhases.READY) {
    return null;
  }

  if (phase === uploadFlowPhases.SESSION_LOADING) {
    return (
      <section className={`status-panel ${statusToneClasses(phase)}`} role="status">
        <p className="status-panel-title">Preparing session</p>
        <p className="mt-1 text-sm text-cyan-50">Creating an anonymous session so your upload can be processed.</p>
      </section>
    );
  }

  if (phase === uploadFlowPhases.UPLOADING) {
    return (
      <section className={`status-panel ${statusToneClasses(phase)}`} role="status">
        <p className="status-panel-title">Uploading media</p>
        <p className="mt-1 text-sm text-cyan-50">Upload received. Waiting for server confirmation.</p>
      </section>
    );
  }

  if (phase === uploadFlowPhases.SUCCESS) {
    const normalizedStatus = typeof jobStatus === "string" && jobStatus.trim().length > 0 ? jobStatus : "QUEUED";
    const progress = clampProgress(jobProgress);
    const stage = formatJobStage(jobStage);
    const hasJobError = jobError && (jobError.message || jobError.code);

    if (normalizedStatus === "FAILED") {
      return (
        <section className={`status-panel ${statusToneClasses(uploadFlowPhases.ERROR)}`} role="alert">
          <p className="status-panel-title">Processing failed</p>
          <p className="mt-1 text-sm text-rose-50">
            {jobError && jobError.message ? jobError.message : "Processing could not be completed."}
          </p>
          {jobError && jobError.code ? <p className="mt-1 text-xs text-rose-100/80">Code: {jobError.code}</p> : null}
          <p className="mt-1 text-xs text-rose-100/80">Stage: {stage}</p>
          <p className="mt-1 text-xs text-rose-100/80">Progress: {progress}%</p>
          {finishedAt ? <p className="mt-1 text-xs text-rose-100/80">Finished at: {finishedAt}</p> : null}
          <JobIdentity jobId={jobId} uploadId={uploadId} />
          <div className="mt-4 flex flex-col gap-2 sm:flex-row">
            <button
              className="min-h-11 rounded-lg border border-rose-300/30 px-3 py-2 text-sm font-semibold text-rose-50 transition hover:bg-rose-500/20"
              onClick={onChooseAnotherFile}
              type="button"
            >
              Choose another file
            </button>
            <button
              className="min-h-11 rounded-lg bg-rose-300 px-3 py-2 text-sm font-semibold text-rose-950 transition hover:bg-rose-200 disabled:cursor-not-allowed disabled:bg-rose-200/30 disabled:text-rose-100"
              disabled={!canRetry}
              onClick={onRetry}
              type="button"
            >
              {isRetrying ? "Retrying..." : "Retry processing"}
            </button>
          </div>
          {error && error.message ? <p className="mt-2 text-xs text-rose-100/90">Retry failed: {error.message}</p> : null}
          {error && error.code ? <p className="mt-1 text-xs text-rose-100/80">Retry code: {error.code}</p> : null}
        </section>
      );
    }

    if (normalizedStatus === "SUCCEEDED") {
      return (
        <section className={`status-panel ${statusToneClasses(uploadFlowPhases.SUCCESS)}`} role="status">
          <p className="status-panel-title">Processing complete</p>
          <p className="mt-1 text-sm text-emerald-50">Appraisal extraction finished successfully.</p>
          {finishedAt ? <p className="mt-2 text-xs text-emerald-100/80">Finished at: {finishedAt}</p> : null}
          <JobIdentity jobId={jobId} uploadId={uploadId} />
          <div className="mt-4">
            <button
              className="min-h-11 rounded-lg border border-emerald-300/30 px-3 py-2 text-sm font-semibold text-emerald-50 transition hover:bg-emerald-500/20"
              onClick={onChooseAnotherFile}
              type="button"
            >
              Choose another file
            </button>
          </div>
        </section>
      );
    }

    if (normalizedStatus === "PENDING_USER_DEDUP") {
      return (
        <section className="status-panel border-amber-500/40 bg-amber-500/10 text-amber-100" role="status">
          <p className="status-panel-title">Waiting for your species selection</p>
          <p className="mt-1 text-sm text-amber-50">
            Processing finished, but we need a species confirmation before finalizing results.
          </p>
          <JobIdentity jobId={jobId} uploadId={uploadId} />
          <div className="mt-4">
            <button
              className="min-h-11 rounded-lg border border-amber-300/30 px-3 py-2 text-sm font-semibold text-amber-50 transition hover:bg-amber-500/20"
              onClick={onChooseAnotherFile}
              type="button"
            >
              Choose another file
            </button>
          </div>
        </section>
      );
    }

    return (
      <section className={`status-panel ${statusToneClasses(phase)}`} role="status">
        <p className="status-panel-title">{normalizedStatus === "QUEUED" ? "Queued for processing" : "Processing"}</p>
        <p className="mt-1 text-sm text-emerald-50">
          {normalizedStatus === "QUEUED"
            ? "Your upload is queued and will start as soon as a worker is available."
            : "Appraisal extraction is currently running."}
        </p>
        <p className="mt-2 text-xs text-emerald-100/90">Stage: {stage}</p>
        <p className="mt-1 text-xs text-emerald-100/90">Progress: {progress}%</p>
        <div aria-label="Job progress" aria-valuemax={100} aria-valuemin={0} aria-valuenow={progress} className="mt-2 h-2 rounded-full bg-emerald-900/60" role="progressbar">
          <div className="h-full rounded-full bg-emerald-300 transition-[width]" style={{ width: `${progress}%` }} />
        </div>
        <JobIdentity jobId={jobId} uploadId={uploadId} />
        {lastPolledAt ? <p className="mt-2 text-xs text-emerald-100/70">Last updated: {lastPolledAt}</p> : null}
        {error && error.message ? (
          <p className="mt-2 text-xs text-amber-100/90">Status check issue: {error.message}. Retrying automatically.</p>
        ) : null}
        {hasJobError && normalizedStatus !== "FAILED" ? (
          <p className="mt-2 text-xs text-amber-100/90">Reported error: {jobError.message || jobError.code}</p>
        ) : null}
        <div className="mt-4">
          <button
            className="min-h-11 rounded-lg border border-emerald-300/30 px-3 py-2 text-sm font-semibold text-emerald-50 transition hover:bg-emerald-500/20"
            onClick={onChooseAnotherFile}
            type="button"
          >
            Choose another file
          </button>
        </div>
      </section>
    );
  }

  return (
    <section className={`status-panel ${statusToneClasses(uploadFlowPhases.ERROR)}`} role="alert">
      <p className="status-panel-title">Upload failed</p>
      <p className="mt-1 text-sm text-rose-50">{error ? error.message : "Please try again."}</p>
      {error && error.code ? <p className="mt-1 text-xs text-rose-100/80">Code: {error.code}</p> : null}
      {error && error.debugMessage ? <p className="mt-1 text-xs text-rose-100/80">Debug: {error.debugMessage}</p> : null}
      <div className="mt-4 flex flex-col gap-2 sm:flex-row">
        <button
          className="min-h-11 rounded-lg border border-rose-300/30 px-3 py-2 text-sm font-semibold text-rose-50 transition hover:bg-rose-500/20"
          onClick={onChooseAnotherFile}
          type="button"
        >
          Choose another file
        </button>
        <button
          className="min-h-11 rounded-lg bg-rose-300 px-3 py-2 text-sm font-semibold text-rose-950 transition hover:bg-rose-200 disabled:cursor-not-allowed disabled:bg-rose-200/30 disabled:text-rose-100"
          disabled={!canRetry}
          onClick={onRetry}
          type="button"
        >
          Retry upload
        </button>
      </div>
    </section>
  );
}
