import { pokemonResultsPhases } from "./pokemon-results-state";

function formatOptionalNumber(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return String(value);
}

function formatConfidence(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return `${Math.round(value * 100)}%`;
}

function formatLevel(level) {
  if (!level || typeof level !== "object") {
    return "N/A";
  }

  const method = typeof level.method === "string" && level.method.trim().length > 0 ? level.method : "UNKNOWN";
  const estimate = formatOptionalNumber(level.estimate);
  const confidence = formatConfidence(level.confidence);

  return `${estimate} (${method}, ${confidence})`;
}

function formatSourceType(sourceType) {
  if (sourceType === "VIDEO") {
    return "Video";
  }

  if (sourceType === "IMAGE") {
    return "Image";
  }

  return sourceType || "Unknown";
}

function formatTimeRange(timeRangeMs) {
  if (!timeRangeMs || typeof timeRangeMs !== "object") {
    return "N/A";
  }

  if (typeof timeRangeMs.start === "number" && typeof timeRangeMs.end === "number") {
    return `${timeRangeMs.start}-${timeRangeMs.end} ms`;
  }

  return "N/A";
}

function formatSourceContext(source) {
  if (!source || typeof source !== "object") {
    return "N/A";
  }

  const type = formatSourceType(source.type);
  const uploadId = source.uploadId || "N/A";
  const jobId = source.jobId || "N/A";
  const timeRange = formatTimeRange(source.timeRangeMs);
  const frameTimestamp =
    typeof source.frameTimestampMs === "number" && !Number.isNaN(source.frameTimestampMs)
      ? `${source.frameTimestampMs} ms`
      : "N/A";

  return `${type} | Upload ${uploadId} | Job ${jobId} | Time ${timeRange} | Frame ${frameTimestamp}`;
}

function formatIVs(ivs) {
  if (!ivs || typeof ivs !== "object") {
    return "N/A";
  }

  const attack = formatOptionalNumber(ivs.attack);
  const defense = formatOptionalNumber(ivs.defense);
  const stamina = formatOptionalNumber(ivs.stamina);

  return `${attack}/${defense}/${stamina}`;
}

function formatPendingOptionHint(option) {
  if (!option || typeof option !== "object") {
    return "";
  }

  const mode = typeof option.matchMode === "string" && option.matchMode.trim().length > 0 ? option.matchMode : "unknown";
  const distance = typeof option.matchDistance === "number" && !Number.isNaN(option.matchDistance) ? option.matchDistance : null;
  const rank = typeof option.optionRank === "number" && !Number.isNaN(option.optionRank) ? option.optionRank : null;

  if (distance === null && rank === null) {
    return mode;
  }

  if (distance === null) {
    return `${mode}, rank ${rank}`;
  }

  if (rank === null) {
    return `${mode}, distance ${distance}`;
  }

  return `${mode}, distance ${distance}, rank ${rank}`;
}

function PendingReadingCard({ onResolvePendingOption, reading, resolving }) {
  return (
    <article className="rounded-xl border border-amber-700/70 bg-amber-950/40 p-4">
      <h3 className="text-sm font-semibold text-amber-100">Reading {reading.id}</h3>
      <p className="mt-1 text-xs text-amber-200/90">
        Job {reading.jobId} | Upload {reading.uploadId}
      </p>
      <p className="mt-1 text-xs text-amber-200/90">
        CP {reading.cp} | HP {reading.hp} | IVs {formatIVs(reading.ivs)}
      </p>
      <p className="mt-1 text-xs text-amber-200/90">
        Level {formatLevel(reading.level)} | Source {formatSourceType(reading.source?.type)} | Frame{" "}
        {typeof reading.source?.frameTimestampMs === "number" ? `${reading.source.frameTimestampMs} ms` : "N/A"}
      </p>
      <p className="mt-1 text-xs text-amber-200/90">Status {reading.status}</p>

      <div className="mt-3 space-y-2">
        {Array.isArray(reading.options)
          ? reading.options.map((option) => (
              <button
                className="block min-h-11 w-full rounded-lg border border-amber-300/30 bg-amber-500/10 px-3 py-2 text-left text-sm text-amber-50 transition hover:bg-amber-500/20 disabled:cursor-not-allowed disabled:opacity-60"
                disabled={resolving}
                key={option.id}
                onClick={() => {
                  onResolvePendingOption(reading.id, option.id);
                }}
                type="button"
              >
                <span className="font-semibold">{option.speciesName}</span>
                <span className="mt-1 block text-xs text-amber-200/80">{formatPendingOptionHint(option)}</span>
              </button>
            ))
          : null}
      </div>
    </article>
  );
}

function ResultCard({ result }) {
  return (
    <article className="rounded-xl border border-slate-800 bg-slate-950/70 p-4">
      <h3 className="text-base font-semibold text-slate-100">{result.speciesName}</h3>
      <p className="mt-1 text-xs text-slate-400">Result ID: {result.id}</p>
      <dl className="mt-3 grid grid-cols-2 gap-2 text-sm text-slate-200">
        <div>
          <dt className="text-xs uppercase tracking-wide text-slate-400">CP</dt>
          <dd>{result.cp}</dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-slate-400">HP</dt>
          <dd>{result.hp}</dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-slate-400">Stardust</dt>
          <dd>{result.powerUpStardustCost}</dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-slate-400">IVs</dt>
          <dd>{formatIVs(result.ivs)}</dd>
        </div>
        <div className="col-span-2">
          <dt className="text-xs uppercase tracking-wide text-slate-400">Level</dt>
          <dd>{formatLevel(result.level)}</dd>
        </div>
        <div className="col-span-2">
          <dt className="text-xs uppercase tracking-wide text-slate-400">Source</dt>
          <dd className="break-all">{formatSourceContext(result.source)}</dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-slate-400">Confidence</dt>
          <dd>{formatConfidence(result.confidence)}</dd>
        </div>
        <div>
          <dt className="text-xs uppercase tracking-wide text-slate-400">Created</dt>
          <dd className="break-all">{result.createdAt}</dd>
        </div>
      </dl>
    </article>
  );
}

export default function PokemonResultsPanel({
  error,
  lastFetchedAt,
  onRetry,
  onResolvePendingOption = () => {},
  pendingReadings = [],
  phase,
  pendingResolveError = null,
  resolvingReadingIds = [],
  results,
}) {
  const normalizedResults = Array.isArray(results) ? results : [];
  const normalizedPendingReadings = Array.isArray(pendingReadings) ? pendingReadings : [];
  const resolvingReadingIDSet = new Set(Array.isArray(resolvingReadingIds) ? resolvingReadingIds : []);
  const hasResults = normalizedResults.length > 0;
  const hasPendingReadings = normalizedPendingReadings.length > 0;
  const isLoading = phase === pokemonResultsPhases.LOADING;
  const isError = phase === pokemonResultsPhases.ERROR;

  return (
    <section className="mt-5 rounded-xl border border-slate-800 bg-slate-950/60 p-4 text-sm text-slate-300">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-slate-100">Accepted Pokemon Results</h2>
        <span className="text-xs text-slate-400">{lastFetchedAt ? `Last updated: ${lastFetchedAt}` : "Not synced yet"}</span>
      </div>

      {isLoading && !hasResults ? (
        <div className="mt-3 rounded-lg border border-cyan-500/40 bg-cyan-500/10 p-3 text-cyan-100" role="status">
          Loading accepted appraisals...
        </div>
      ) : null}

      {!isLoading && isError && !hasResults ? (
        <div className="mt-3 rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-rose-100" role="alert">
          <p>{error && error.message ? error.message : "Could not load accepted appraisals."}</p>
          <button
            className="mt-3 rounded-lg border border-rose-300/40 px-3 py-2 text-sm font-semibold text-rose-50 transition hover:bg-rose-500/20"
            onClick={onRetry}
            type="button"
          >
            Retry results
          </button>
        </div>
      ) : null}

      {!isLoading && !isError && !hasResults ? (
        <div className="mt-3 rounded-lg border border-slate-700 bg-slate-900/80 p-3 text-slate-300" role="status">
          No accepted appraisals yet. Upload a screenshot or video and wait for processing to finish.
        </div>
      ) : null}

      {hasPendingReadings ? (
        <div className="mt-4 rounded-lg border border-amber-600/40 bg-amber-950/20 p-3">
          <h3 className="text-sm font-semibold uppercase tracking-wide text-amber-100">Pending Species Confirmation</h3>
          <p className="mt-1 text-xs text-amber-200/90">
            Choose one species option for each pending reading to finalize results.
          </p>
          {pendingResolveError && pendingResolveError.message ? (
            <p className="mt-2 text-xs text-rose-200" role="alert">
              {pendingResolveError.message}
            </p>
          ) : null}
          <div className="mt-3 grid gap-3">
            {normalizedPendingReadings.map((reading) => (
              <PendingReadingCard
                key={reading.id}
                onResolvePendingOption={onResolvePendingOption}
                reading={reading}
                resolving={resolvingReadingIDSet.has(reading.id)}
              />
            ))}
          </div>
        </div>
      ) : null}

      {hasResults ? (
        <div className="mt-4 space-y-4">
          {isLoading ? (
            <p className="text-xs text-cyan-200" role="status">
              Refreshing accepted appraisals...
            </p>
          ) : null}
          {isError ? (
            <div className="rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-rose-100" role="alert">
              <p>{error && error.message ? error.message : "Could not refresh accepted appraisals."}</p>
              <button
                className="mt-3 rounded-lg border border-rose-300/40 px-3 py-2 text-sm font-semibold text-rose-50 transition hover:bg-rose-500/20"
                onClick={onRetry}
                type="button"
              >
                Retry results
              </button>
            </div>
          ) : null}

          <div className="grid gap-3 md:hidden">
            {normalizedResults.map((result) => (
              <ResultCard key={result.id} result={result} />
            ))}
          </div>

          <div className="hidden overflow-x-auto md:block">
            <table className="min-w-full border-collapse text-left text-xs">
              <thead className="text-slate-400">
                <tr className="border-b border-slate-800">
                  <th className="px-2 py-2">Species</th>
                  <th className="px-2 py-2">CP</th>
                  <th className="px-2 py-2">HP</th>
                  <th className="px-2 py-2">Stardust</th>
                  <th className="px-2 py-2">IVs</th>
                  <th className="px-2 py-2">Level</th>
                  <th className="px-2 py-2">Source</th>
                  <th className="px-2 py-2">Confidence</th>
                  <th className="px-2 py-2">Created</th>
                </tr>
              </thead>
              <tbody>
                {normalizedResults.map((result) => (
                  <tr className="border-b border-slate-900 align-top text-slate-200" key={result.id}>
                    <td className="px-2 py-2">
                      <p className="font-medium text-slate-100">{result.speciesName}</p>
                      <p className="text-[11px] text-slate-500">{result.id}</p>
                    </td>
                    <td className="px-2 py-2">{result.cp}</td>
                    <td className="px-2 py-2">{result.hp}</td>
                    <td className="px-2 py-2">{result.powerUpStardustCost}</td>
                    <td className="px-2 py-2">{formatIVs(result.ivs)}</td>
                    <td className="px-2 py-2">{formatLevel(result.level)}</td>
                    <td className="px-2 py-2 break-all">{formatSourceContext(result.source)}</td>
                    <td className="px-2 py-2">{formatConfidence(result.confidence)}</td>
                    <td className="px-2 py-2 break-all">{result.createdAt}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
    </section>
  );
}
