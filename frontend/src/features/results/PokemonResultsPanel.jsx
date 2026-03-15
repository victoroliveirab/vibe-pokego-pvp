import { Fragment, useState } from "react";
import { pokemonResultsPhases } from "./pokemon-results-state";

const leagueTabs = [
  { key: "little", label: "Little" },
  { key: "great", label: "Great" },
  { key: "ultra", label: "Ultra" },
];

function formatOptionalNumber(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return String(value);
}

function formatPercentage(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return `${value.toFixed(2)}%`;
}

function formatDecimal(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return value.toFixed(2);
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

function normalizeMaxCPEvaluations(maxCPEvaluations) {
  return Array.isArray(maxCPEvaluations) ? maxCPEvaluations : [];
}

function isValidRank(rank) {
  return typeof rank === "number" && !Number.isNaN(rank) && rank > 0;
}

function rankToTier(rank) {
  if (!isValidRank(rank)) {
    return "F";
  }

  if (rank <= 10) {
    return "S";
  }
  if (rank <= 50) {
    return "A";
  }
  if (rank <= 100) {
    return "B";
  }
  if (rank <= 200) {
    return "C";
  }
  if (rank <= 400) {
    return "D";
  }
  return "F";
}

function tierChipClasses(tier) {
  if (tier === "S") {
    return "border-emerald-300/70 bg-emerald-500/20 text-emerald-100";
  }
  if (tier === "A") {
    return "border-cyan-300/70 bg-cyan-500/20 text-cyan-100";
  }
  if (tier === "B") {
    return "border-blue-300/70 bg-blue-500/20 text-blue-100";
  }
  if (tier === "C") {
    return "border-amber-300/70 bg-amber-500/20 text-amber-100";
  }
  if (tier === "D") {
    return "border-orange-300/70 bg-orange-500/20 text-orange-100";
  }
  if (tier === "F") {
    return "border-rose-300/70 bg-rose-500/20 text-rose-100";
  }
  return "border-slate-500/70 bg-slate-700/30 text-slate-200";
}

function mapMaxCPToLeague(maxCP) {
  if (maxCP === 500) {
    return "little";
  }
  if (maxCP === 1500) {
    return "great";
  }
  if (maxCP === 2500) {
    return "ultra";
  }
  return null;
}

function formatSpeciesDisplayName(speciesId) {
  if (typeof speciesId !== "string" || speciesId.trim().length === 0) {
    return "Unknown";
  }

  return speciesId
    .trim()
    .replace(/[\-_]+/g, " ")
    .replace(/\s+/g, " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

function compareLeagueEntries(left, right) {
  const leftRank = isValidRank(left.rank) ? left.rank : Number.POSITIVE_INFINITY;
  const rightRank = isValidRank(right.rank) ? right.rank : Number.POSITIVE_INFINITY;

  if (leftRank !== rightRank) {
    return leftRank - rightRank;
  }

  if (left.percentage !== right.percentage) {
    return right.percentage - left.percentage;
  }

  return String(left.evaluatedSpeciesId).localeCompare(String(right.evaluatedSpeciesId));
}

function buildLeagueBreakdown(maxCPEvaluations) {
  const byLeague = {
    little: [],
    great: [],
    ultra: [],
  };

  const entries = normalizeMaxCPEvaluations(maxCPEvaluations);
  let bestRank = null;

  for (const entry of entries) {
    const league = mapMaxCPToLeague(entry.maxCp);
    if (!league) {
      continue;
    }

    byLeague[league].push({
      ...entry,
      league,
      tier: rankToTier(entry.rank),
      speciesDisplayName: formatSpeciesDisplayName(entry.evaluatedSpeciesId),
    });

    if (isValidRank(entry.rank) && (bestRank === null || entry.rank < bestRank)) {
      bestRank = entry.rank;
    }
  }

  for (const tab of leagueTabs) {
    byLeague[tab.key].sort(compareLeagueEntries);
  }

  return {
    byLeague,
    bestAvailableTier: bestRank === null ? null : rankToTier(bestRank),
  };
}

function selectDefaultLeagueTab(byLeague) {
  if (Array.isArray(byLeague.great) && byLeague.great.length > 0) {
    return "great";
  }
  if (Array.isArray(byLeague.little) && byLeague.little.length > 0) {
    return "little";
  }
  if (Array.isArray(byLeague.ultra) && byLeague.ultra.length > 0) {
    return "ultra";
  }
  return "great";
}

function hasLeagueEntries(byLeague) {
  return leagueTabs.some((tab) => Array.isArray(byLeague[tab.key]) && byLeague[tab.key].length > 0);
}

function deriveBestFitEvaluation(maxCPEvaluations) {
  const entries = normalizeMaxCPEvaluations(maxCPEvaluations);
  if (entries.length === 0) {
    return null;
  }

  const sorted = [...entries].sort((left, right) => {
    if (left.percentage !== right.percentage) {
      return right.percentage - left.percentage;
    }
    if (left.rank !== right.rank) {
      return left.rank - right.rank;
    }
    if (left.maxCp !== right.maxCp) {
      return right.maxCp - left.maxCp;
    }
    return 0;
  });

  return sorted[0];
}

function formatBestFitSummary(maxCPEvaluations) {
  const bestFit = deriveBestFitEvaluation(maxCPEvaluations);
  if (!bestFit) {
    return "Best fit: N/A";
  }

  return `Best fit: ${bestFit.evaluatedSpeciesId} @ ${bestFit.maxCp} CP (${formatPercentage(bestFit.percentage)}, rank ${bestFit.rank})`;
}

function TierChip({ ariaLabel, tier }) {
  const displayTier = typeof tier === "string" && tier.trim().length > 0 ? tier : "N/A";

  return (
    <span
      aria-label={ariaLabel}
      className={`inline-flex min-w-8 items-center justify-center rounded-full border px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide ${tierChipClasses(
        displayTier,
      )}`}
    >
      {displayTier}
    </span>
  );
}

function LeagueBreakdownPanel({ activeLeague, byLeague, onSelectLeague, regionLabel }) {
  const entries = Array.isArray(byLeague[activeLeague]) ? byLeague[activeLeague] : [];

  return (
    <section aria-label={regionLabel} className="rounded-lg border border-slate-700 bg-slate-900/70 p-3" role="region">
      <div className="grid grid-cols-3 gap-1.5 sm:flex sm:flex-wrap sm:gap-2" role="tablist">
        {leagueTabs.map((tab) => {
          const count = Array.isArray(byLeague[tab.key]) ? byLeague[tab.key].length : 0;
          const isActive = activeLeague === tab.key;

          return (
            <button
              aria-pressed={isActive}
              className={`w-full rounded-full border px-2 py-1 text-[11px] font-semibold transition sm:w-auto sm:px-3 sm:text-xs ${
                isActive
                  ? "border-slate-200 bg-slate-100 text-slate-900"
                  : "border-slate-600 bg-slate-800/60 text-slate-300 hover:bg-slate-700"
              }`}
              key={tab.key}
              onClick={() => {
                onSelectLeague(tab.key);
              }}
              role="tab"
              type="button"
            >
              {tab.label} ({count})
            </button>
          );
        })}
      </div>

      {entries.length === 0 ? (
        <p className="mt-3 text-xs text-slate-400">No evaluations for this league.</p>
      ) : (
        <ol className="mt-3 space-y-2">
          {entries.map((entry) => (
            <li className="rounded-lg border border-slate-700 bg-slate-900 p-3" key={`${entry.evaluatedSpeciesId}-${entry.maxCp}-${entry.rank}`}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-sm font-semibold text-slate-100">
                  {entry.speciesDisplayName}
                  <span className="ml-1 text-xs text-slate-400">({entry.evaluatedSpeciesId})</span>
                </p>
                <TierChip ariaLabel={`Tier ${entry.tier} for ${entry.speciesDisplayName}`} tier={entry.tier} />
              </div>

              <dl className="mt-2 grid grid-cols-2 gap-2 text-xs text-slate-200 md:grid-cols-4">
                <div>
                  <dt className="uppercase tracking-wide text-slate-400">Target CP</dt>
                  <dd>{entry.maxCp}</dd>
                </div>
                <div>
                  <dt className="uppercase tracking-wide text-slate-400">Rank</dt>
                  <dd>{formatPercentage(entry.percentage)}</dd>
                </div>
                <div>
                  <dt className="uppercase tracking-wide text-slate-400">Position</dt>
                  <dd>{isValidRank(entry.rank) ? `#${entry.rank}` : "N/A"}</dd>
                </div>
                <div>
                  <dt className="uppercase tracking-wide text-slate-400">Level</dt>
                  <dd>{formatDecimal(entry.bestLevel)}</dd>
                </div>
              </dl>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
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
  const maxCPEvaluations = normalizeMaxCPEvaluations(result.maxCpEvaluations);
  const leagueBreakdown = buildLeagueBreakdown(maxCPEvaluations);
  const [isBreakdownOpen, setIsBreakdownOpen] = useState(false);
  const [activeLeague, setActiveLeague] = useState(() => selectDefaultLeagueTab(leagueBreakdown.byLeague));
  const hasBreakdownEntries = hasLeagueEntries(leagueBreakdown.byLeague);
  const bestTier = leagueBreakdown.bestAvailableTier || "N/A";

  return (
    <article className="rounded-xl border border-slate-800 bg-slate-950/70 p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-base font-semibold text-slate-100">{result.speciesName}</h3>
        <TierChip ariaLabel={`Best tier for card ${result.speciesName}: ${bestTier}`} tier={bestTier} />
      </div>
      <p className="mt-1 text-xs text-slate-400">Result ID: {result.id}</p>
      <p className="mt-2 text-xs text-emerald-200">{formatBestFitSummary(maxCPEvaluations)}</p>
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

      {hasBreakdownEntries ? (
        <div className="mt-3">
          <button
            aria-expanded={isBreakdownOpen}
            className="min-h-10 rounded-lg border border-slate-600 bg-slate-800/80 px-3 py-2 text-xs font-semibold text-slate-100 transition hover:bg-slate-700"
            onClick={() => {
              setIsBreakdownOpen((current) => !current);
            }}
            type="button"
          >
            {isBreakdownOpen ? "Hide League Breakdown" : "Show League Breakdown"}
          </button>

          {isBreakdownOpen ? (
            <div className="mt-2">
              <LeagueBreakdownPanel
                activeLeague={activeLeague}
                byLeague={leagueBreakdown.byLeague}
                onSelectLeague={setActiveLeague}
                regionLabel={`League breakdown card for ${result.speciesName}`}
              />
            </div>
          ) : null}
        </div>
      ) : null}
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
  const [expandedResultIDSet, setExpandedResultIDSet] = useState(() => new Set());
  const [rowActiveLeagueByID, setRowActiveLeagueByID] = useState({});

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
                  <th className="px-2 py-2">Details</th>
                  <th className="px-2 py-2">Species</th>
                  <th className="px-2 py-2">CP</th>
                  <th className="px-2 py-2">HP</th>
                  <th className="px-2 py-2">Stardust</th>
                  <th className="px-2 py-2">IVs</th>
                  <th className="px-2 py-2">Level</th>
                  <th className="px-2 py-2">Best Fit</th>
                  <th className="px-2 py-2">Source</th>
                  <th className="px-2 py-2">Confidence</th>
                  <th className="px-2 py-2">Created</th>
                </tr>
              </thead>
              <tbody>
                {normalizedResults.map((result) => {
                  const maxCPEvaluations = normalizeMaxCPEvaluations(result.maxCpEvaluations);
                  const leagueBreakdown = buildLeagueBreakdown(maxCPEvaluations);
                  const hasBreakdownEntries = hasLeagueEntries(leagueBreakdown.byLeague);
                  const bestTier = leagueBreakdown.bestAvailableTier || "N/A";
                  const defaultLeague = selectDefaultLeagueTab(leagueBreakdown.byLeague);
                  const activeLeague = rowActiveLeagueByID[result.id] || defaultLeague;
                  const isExpanded = expandedResultIDSet.has(result.id);

                  return (
                    <Fragment key={result.id}>
                      <tr className="border-b border-slate-900 align-top text-slate-200">
                        <td className="px-2 py-2">
                          <button
                            aria-controls={`league-breakdown-${result.id}`}
                            aria-expanded={isExpanded}
                            aria-label={`Toggle league breakdown row for ${result.speciesName}`}
                            className="inline-flex min-h-9 min-w-9 items-center justify-center rounded-md border border-slate-600 bg-slate-800/70 text-slate-100 transition hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-50"
                            disabled={!hasBreakdownEntries}
                            onClick={() => {
                              setExpandedResultIDSet((current) => {
                                const next = new Set(current);
                                if (next.has(result.id)) {
                                  next.delete(result.id);
                                } else {
                                  next.add(result.id);
                                }
                                return next;
                              });

                              setRowActiveLeagueByID((current) => {
                                if (current[result.id]) {
                                  return current;
                                }
                                return {
                                  ...current,
                                  [result.id]: defaultLeague,
                                };
                              });
                            }}
                            type="button"
                          >
                            {isExpanded ? "v" : ">"}
                          </button>
                        </td>
                        <td className="px-2 py-2">
                          <div className="flex flex-wrap items-center gap-2">
                            <p className="font-medium text-slate-100">{result.speciesName}</p>
                            <TierChip ariaLabel={`Best tier for row ${result.speciesName}: ${bestTier}`} tier={bestTier} />
                          </div>
                          <p className="text-[11px] text-slate-500">{result.id}</p>
                        </td>
                        <td className="px-2 py-2">{result.cp}</td>
                        <td className="px-2 py-2">{result.hp}</td>
                        <td className="px-2 py-2">{result.powerUpStardustCost}</td>
                        <td className="px-2 py-2">{formatIVs(result.ivs)}</td>
                        <td className="px-2 py-2">{formatLevel(result.level)}</td>
                        <td className="px-2 py-2 text-emerald-200">{formatBestFitSummary(maxCPEvaluations)}</td>
                        <td className="px-2 py-2 break-all">{formatSourceContext(result.source)}</td>
                        <td className="px-2 py-2">{formatConfidence(result.confidence)}</td>
                        <td className="px-2 py-2 break-all">{result.createdAt}</td>
                      </tr>

                      {isExpanded ? (
                        <tr className="border-b border-slate-900 bg-slate-900/40" id={`league-breakdown-${result.id}`}>
                          <td className="px-2 pb-3" colSpan={11}>
                            <LeagueBreakdownPanel
                              activeLeague={activeLeague}
                              byLeague={leagueBreakdown.byLeague}
                              onSelectLeague={(league) => {
                                setRowActiveLeagueByID((current) => ({
                                  ...current,
                                  [result.id]: league,
                                }));
                              }}
                              regionLabel={`League breakdown row for ${result.speciesName}`}
                            />
                          </td>
                        </tr>
                      ) : null}
                    </Fragment>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      ) : null}
    </section>
  );
}
