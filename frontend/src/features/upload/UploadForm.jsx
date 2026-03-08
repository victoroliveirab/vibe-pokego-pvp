function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return "0 B";
  }

  const units = ["B", "KB", "MB", "GB"];
  const unitIndex = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
  const value = bytes / 1024 ** unitIndex;
  const precision = unitIndex === 0 ? 0 : 1;

  return `${value.toFixed(precision)} ${units[unitIndex]}`;
}

export default function UploadForm({
  canSubmit,
  isSessionLoading,
  isSubmitting,
  onSelectFile,
  onSubmit,
  selectedFile,
}) {
  const buttonLabel = isSessionLoading ? "Preparing session..." : isSubmitting ? "Uploading..." : "Submit Upload";

  return (
    <form className="space-y-4" onSubmit={onSubmit}>
      <label className="block text-sm font-medium text-slate-200" htmlFor="media-upload">
        Choose image or video
      </label>

      <input
        accept="image/*,video/*"
        className="block w-full cursor-pointer rounded-xl border border-slate-700 bg-slate-950 px-3 py-3 text-sm text-slate-200 file:mr-4 file:min-h-11 file:cursor-pointer file:rounded-lg file:border-0 file:bg-cyan-500 file:px-4 file:py-2 file:text-sm file:font-semibold file:text-slate-950 hover:file:bg-cyan-300"
        id="media-upload"
        onChange={(event) => onSelectFile(event.target.files && event.target.files.length > 0 ? event.target.files[0] : null)}
        type="file"
      />

      <p className="rounded-lg border border-slate-800 bg-slate-950/40 px-3 py-2 text-xs text-slate-300">
        {selectedFile ? `Selected: ${selectedFile.name} (${formatBytes(selectedFile.size)})` : "No file selected yet."}
      </p>

      <button
        className="min-h-11 w-full rounded-xl bg-cyan-400 px-4 py-3 text-sm font-semibold text-slate-950 transition hover:bg-cyan-300 disabled:cursor-not-allowed disabled:bg-slate-700 disabled:text-slate-300"
        disabled={!canSubmit}
        type="submit"
      >
        {buttonLabel}
      </button>
    </form>
  );
}
