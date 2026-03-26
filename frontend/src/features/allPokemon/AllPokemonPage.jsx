import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { getUserFacingErrorMessage } from "../../lib/api-errors";
import { useAppIdentity } from "../session/useAppIdentity";
import { tierChipClasses } from "../results/pokemonLeagueDisplayUtils";
import { createPokemonResultsApi } from "../results/pokemon-results-api";
import {
  buildAllPokemonRow,
  dedupePokemonResults,
  formatCreatedAt,
  formatSpeciesName,
  isValidRank,
  leagueTabs,
  sortOptions,
} from "./allPokemonUtils";

const allPokemonPhases = {
  IDLE: "idle",
  LOADING: "loading",
  SUCCESS: "success",
  ERROR: "error",
};
const initialDeleteModalState = {
  isOpen: false,
  resultId: "",
  speciesName: "",
};

function normalizePokemonResultsError(error) {
  const code =
    error && typeof error === "object" && typeof error.code === "string" ? error.code : "POKEMON_RESULTS_FAILED";

  return {
    code,
    message: getUserFacingErrorMessage(error),
  };
}

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

function formatCompactPercentage(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return `${value.toFixed(1)}%`;
}

function formatConfidence(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return `${Math.round(value * 100)}%`;
}

function formatLevelEstimate(level) {
  if (!level || typeof level !== "object") {
    return "N/A";
  }

  return formatOptionalNumber(level.estimate);
}

function formatCompactLevel(level) {
  if (!level || typeof level !== "object" || typeof level.estimate !== "number" || Number.isNaN(level.estimate)) {
    return "N/A";
  }

  return level.estimate.toFixed(1);
}

function formatIVs(ivs) {
  if (!ivs || typeof ivs !== "object") {
    return "N/A";
  }

  return `${formatOptionalNumber(ivs.attack)}/${formatOptionalNumber(ivs.defense)}/${formatOptionalNumber(ivs.stamina)}`;
}

function formatCompactIVs(ivs) {
  if (!ivs || typeof ivs !== "object") {
    return "N/A";
  }

  return `${formatOptionalNumber(ivs.attack)}-${formatOptionalNumber(ivs.defense)}-${formatOptionalNumber(ivs.stamina)}`;
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

  const sourceParts = [formatSourceType(source.type)];

  if (source.uploadId) {
    sourceParts.push(`Upload ${source.uploadId}`);
  }
  if (source.jobId) {
    sourceParts.push(`Job ${source.jobId}`);
  }

  sourceParts.push(`Time ${formatTimeRange(source.timeRangeMs)}`);
  sourceParts.push(
    `Frame ${typeof source.frameTimestampMs === "number" && !Number.isNaN(source.frameTimestampMs) ? `${source.frameTimestampMs} ms` : "N/A"}`,
  );

  return sourceParts.join(" | ");
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

function DeleteResultDialog({
  error,
  isDeleting = false,
  isOpen = false,
  onCancel,
  onConfirm,
  speciesName = "this result",
}) {
  const cancelButtonRef = useRef(null);

  useEffect(() => {
    if (!isOpen) {
      return undefined;
    }

    cancelButtonRef.current?.focus();

    function handleKeyDown(event) {
      if (event.key === "Escape" && !isDeleting) {
        onCancel();
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => {
      window.removeEventListener("keydown", handleKeyDown);
    };
  }, [isDeleting, isOpen, onCancel]);

  if (!isOpen) {
    return null;
  }

  return (
    <div
      aria-modal="true"
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/80 px-4"
      onClick={(event) => {
        if (event.target === event.currentTarget && !isDeleting) {
          onCancel();
        }
      }}
      role="dialog"
    >
      <div className="w-full max-w-sm rounded-2xl border border-slate-700 bg-slate-900 p-5 shadow-2xl shadow-slate-950/70">
        <h3 className="text-lg font-semibold text-slate-50">Delete result?</h3>
        <p className="mt-2 text-sm text-slate-300">
          Delete <span className="font-semibold text-slate-100">{speciesName}</span>? This hides the accepted appraisal
          from future lists.
        </p>
        {error && error.message ? (
          <p className="mt-3 rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-sm text-rose-100" role="alert">
            {error.message}
          </p>
        ) : null}
        <div className="mt-5 flex justify-end gap-3">
          <button
            className="min-h-10 rounded-lg border border-slate-600 px-4 py-2 text-sm font-semibold text-slate-100 transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isDeleting}
            onClick={onCancel}
            ref={cancelButtonRef}
            type="button"
          >
            Cancel
          </button>
          <button
            className="min-h-10 rounded-lg border border-rose-300/40 bg-rose-500/10 px-4 py-2 text-sm font-semibold text-rose-100 transition hover:bg-rose-500/20 disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isDeleting}
            onClick={onConfirm}
            type="button"
          >
            {isDeleting ? "Deleting..." : "Delete"}
          </button>
        </div>
      </div>
    </div>
  );
}

function compareRank(a, b, direction) {
  const aRank = isValidRank(a.bestRank) ? a.bestRank : Number.POSITIVE_INFINITY;
  const bRank = isValidRank(b.bestRank) ? b.bestRank : Number.POSITIVE_INFINITY;
  if (aRank !== bRank) {
    return direction === "asc" ? aRank - bRank : bRank - aRank;
  }

  if (a.scanDate !== b.scanDate) {
    return direction === "asc" ? a.scanDate - b.scanDate : b.scanDate - a.scanDate;
  }

  return String(a.result.speciesName).localeCompare(String(b.result.speciesName));
}

function compareDate(a, b, direction) {
  const normalize = (value) => {
    if (Number.isNaN(value)) {
      return direction === "asc" ? Number.POSITIVE_INFINITY : Number.NEGATIVE_INFINITY;
    }
    return value;
  };

  const aDate = normalize(a.scanDate);
  const bDate = normalize(b.scanDate);

  if (aDate !== bDate) {
    return direction === "asc" ? aDate - bDate : bDate - aDate;
  }

  return String(a.result.speciesName).localeCompare(String(b.result.speciesName));
}

export default function AllPokemonPage({
  pokemonResultsApi: injectedPokemonResultsApi = null,
  useSessionHook = useAppIdentity,
}) {
  const identity = useSessionHook();
  const pokemonResultsApi = useMemo(
    () => injectedPokemonResultsApi || createPokemonResultsApi({ apiClient: identity.apiClient }),
    [identity.apiClient, injectedPokemonResultsApi],
  );
  const { sessionId, isLoading: isSessionLoading, error: sessionError } = identity;
  const [phase, setPhase] = useState(allPokemonPhases.IDLE);
  const [error, setError] = useState(null);
  const [results, setResults] = useState([]);
  const [lastFetchedAt, setLastFetchedAt] = useState("");
  const [selectedLeague, setSelectedLeague] = useState("great");
  const [sortMode, setSortMode] = useState(sortOptions[1].key);
  const [isDebugMode, setIsDebugMode] = useState(false);
  const [deleteModalState, setDeleteModalState] = useState(initialDeleteModalState);
  const [pendingDeleteError, setPendingDeleteError] = useState(null);
  const [deletingResultIds, setDeletingResultIds] = useState([]);
  const [expandedResultIDSet, setExpandedResultIDSet] = useState(() => new Set());
  const requestIdRef = useRef(0);
  const previousIdentityModeRef = useRef(identity.mode);

  useEffect(() => {
    const previousIdentityMode = previousIdentityModeRef.current;

    if (previousIdentityMode && previousIdentityMode !== identity.mode) {
      requestIdRef.current += 1;
      setResults([]);
      setError(null);
      setPhase(allPokemonPhases.IDLE);
      setLastFetchedAt("");
      setDeleteModalState(initialDeleteModalState);
      setPendingDeleteError(null);
      setDeletingResultIds([]);
      setExpandedResultIDSet(new Set());
    }

    previousIdentityModeRef.current = identity.mode;
  }, [identity.mode]);

  const loadPokemonResults = useCallback(async () => {
    const normalizedSessionId = typeof sessionId === "string" ? sessionId.trim() : "";
    if (!normalizedSessionId) {
      return;
    }

    const requestId = ++requestIdRef.current;
    setPhase(allPokemonPhases.LOADING);
    setError(null);

    try {
      const payload = await pokemonResultsApi.getPokemonResults({
        sessionId: normalizedSessionId,
      });
      if (requestId !== requestIdRef.current) {
        return;
      }

      setResults(Array.isArray(payload.results) ? payload.results : []);
      setLastFetchedAt(new Date().toISOString());
      setPhase(allPokemonPhases.SUCCESS);
    } catch (error) {
      if (requestId !== requestIdRef.current) {
        return;
      }

      setError(normalizePokemonResultsError(error));
      setPhase(allPokemonPhases.ERROR);
    }
  }, [pokemonResultsApi, sessionId]);

  useEffect(() => {
    if (isSessionLoading) {
      return;
    }

    if (sessionError) {
      setError({
        code: sessionError.code || "SESSION_BOOTSTRAP_FAILED",
        message: getUserFacingErrorMessage(sessionError),
      });
      setResults([]);
      setPhase(allPokemonPhases.ERROR);
      return;
    }

    if (!sessionId) {
      setResults([]);
      setError(null);
      setPhase(allPokemonPhases.IDLE);
      return;
    }

    void loadPokemonResults();
  }, [isSessionLoading, sessionError, sessionId, loadPokemonResults]);

  const dedupedResults = useMemo(() => dedupePokemonResults(results), [results]);

  const rows = useMemo(() => {
    const mapped = dedupedResults
      .map((result) => buildAllPokemonRow(result, selectedLeague))
      .filter((entry) => entry !== null);

    const nextRows = [...mapped];
    if (sortMode === "scanDateAsc") {
      nextRows.sort((left, right) => compareDate(left, right, "asc"));
    } else if (sortMode === "scanDateDesc") {
      nextRows.sort((left, right) => compareDate(left, right, "desc"));
    } else if (sortMode === "rankAsc") {
      nextRows.sort((left, right) => compareRank(left, right, "asc"));
    } else if (sortMode === "rankDesc") {
      nextRows.sort((left, right) => compareRank(left, right, "desc"));
    }

    return nextRows;
  }, [dedupedResults, selectedLeague, sortMode]);

  const deletingResultIDSet = useMemo(() => new Set(deletingResultIds), [deletingResultIds]);
  const selectedLeagueLabel = leagueTabs.find((tab) => tab.key === selectedLeague)?.label || "Selected";
  const isDeleteInFlight = deleteModalState.resultId ? deletingResultIDSet.has(deleteModalState.resultId) : false;

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
      if (deletingResultIDSet.has(current.resultId)) {
        return current;
      }

      return {
        isOpen: false,
        resultId: "",
        speciesName: "",
      };
    });
  }, [deletingResultIDSet]);

  const handleConfirmDeleteResult = useCallback(async () => {
    const normalizedSessionID = typeof sessionId === "string" ? sessionId.trim() : "";
    if (!normalizedSessionID) {
      setPendingDeleteError({
        code: "INVALID_SESSION",
        message: "Your session expired. We can try again.",
      });
      return;
    }

    const normalizedResultID = deleteModalState.resultId.trim();
    if (!normalizedResultID) {
      setPendingDeleteError({
        code: "INVALID_REQUEST",
        message: "Could not delete this result because the request was incomplete.",
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
      setExpandedResultIDSet((current) => {
        const next = new Set(current);
        next.delete(normalizedResultID);
        return next;
      });
      await loadPokemonResults();
    } catch (error) {
      setPendingDeleteError(normalizePokemonResultsError(error));
    } finally {
      setDeletingResultIds((current) => current.filter((id) => id !== normalizedResultID));
    }
  }, [deleteModalState.resultId, loadPokemonResults, pokemonResultsApi, sessionId]);

  return (
    <main className="min-h-screen bg-gradient-to-b from-slate-950 via-slate-900 to-slate-950">
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 pb-10 pt-8 sm:px-6">
        {identity.mode === "guest" ? (
          <section className="rounded-2xl border border-amber-400/30 bg-amber-500/10 p-4 text-amber-50 shadow-xl shadow-slate-950/40 sm:p-5">
            <p className="text-xs font-semibold uppercase tracking-[0.24em]">Guest Records</p>
            <p className="mt-2 text-sm leading-6">
              Guest scans can disappear at any time and will be cleared when you sign in. Sign up to keep scans saved
              and synced across devices.
            </p>
          </section>
        ) : null}

        <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4 shadow-xl shadow-slate-950/60 sm:p-6">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h1 className="text-2xl font-semibold text-white">All Scanned Pokemon</h1>
              <p className="mt-1 max-w-2xl text-sm text-slate-300">
                Browse accepted scans by league, keep only league-relevant picks visible, and manage the list without
                debug noise.
              </p>
            </div>
            <p className="text-xs text-slate-400">{lastFetchedAt ? `Last updated: ${lastFetchedAt}` : "Not synced yet"}</p>
          </div>

          {phase === allPokemonPhases.LOADING && results.length === 0 ? (
            <p className="mt-3 rounded-lg border border-cyan-500/40 bg-cyan-500/10 p-3 text-cyan-100" role="status">
              Loading scanned Pokemon...
            </p>
          ) : null}

          {!isSessionLoading && sessionError ? (
            <p className="mt-3 rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-rose-100" role="alert">
              {getUserFacingErrorMessage(sessionError)}
            </p>
          ) : null}

          <div className="mt-4 flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
            <div className="flex flex-wrap gap-2">
              {leagueTabs.map((tab) => (
                <button
                  key={tab.key}
                  aria-pressed={selectedLeague === tab.key}
                  className={`min-h-10 rounded-full border px-3 py-2 text-xs font-semibold transition ${selectedLeague === tab.key
                    ? "border-slate-200 bg-slate-100 text-slate-900"
                    : "border-slate-600 bg-slate-800/60 text-slate-300 hover:bg-slate-700"
                    }`}
                  onClick={() => {
                    setSelectedLeague(tab.key);
                  }}
                  type="button"
                >
                  {tab.label}
                </button>
              ))}
            </div>

            <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
              <label className="block text-xs font-semibold uppercase tracking-wide text-slate-300" htmlFor="all-pokemon-sort">
                Sort
                <select
                  className="mt-2 min-h-10 w-full rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs text-slate-100 outline-none ring-0 focus:border-slate-300 sm:min-w-72"
                  id="all-pokemon-sort"
                  value={sortMode}
                  onChange={(event) => {
                    setSortMode(event.target.value);
                  }}
                >
                  {sortOptions.map((option) => (
                    <option value={option.key} key={option.key}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          </div>

          {phase === allPokemonPhases.ERROR && rows.length === 0 ? (
            <div className="mt-4 rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-rose-100" role="alert">
              <p>{error && error.message ? error.message : "Could not load scans."}</p>
              <button
                className="mt-2 rounded-lg border border-rose-300/40 px-3 py-2 text-xs font-semibold text-rose-50 transition hover:bg-rose-500/20"
                onClick={loadPokemonResults}
                type="button"
              >
                Retry
              </button>
            </div>
          ) : null}

          {rows.length > 0 ? (
            <section className="mt-4 space-y-3">
              {phase === allPokemonPhases.LOADING ? (
                <p className="text-xs text-cyan-200" role="status">
                  Refreshing scanned Pokemon...
                </p>
              ) : null}
              {rows.map((row) => {
                const result = row.result;
                const speciesName = formatSpeciesName(result.speciesName);
                const isExpanded = expandedResultIDSet.has(result.id);
                const deleting = deletingResultIDSet.has(result.id);
                const bestLeagueLabel = leagueTabs.find((tab) => tab.key === row.bestLeagueEntry?.league)?.label || "N/A";

                return (
                  <article
                    className="rounded-2xl border border-slate-800 bg-slate-950/70 p-4 shadow-lg shadow-slate-950/40"
                    data-testid="scan-row"
                    key={result.id}
                  >
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <h2 className="text-base font-semibold text-slate-100">{speciesName}</h2>
                          <TierChip
                            ariaLabel={`Best ${selectedLeagueLabel} tier for ${speciesName}: ${row.bestTier}`}
                            tier={row.bestTier}
                          />
                        </div>
                        <p className="mt-1 text-xs text-slate-300 sm:text-sm">
                          {`CP ${formatOptionalNumber(result.cp)} / IV ${formatCompactIVs(result.ivs)} / LVL ${formatCompactLevel(result.level)} / ${bestLeagueLabel} ${formatCompactPercentage(
                            row.bestLeagueEntry?.percentage,
                          )}`}
                        </p>
                      </div>
                      <button
                        aria-label={`Delete ${speciesName}`}
                        className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-rose-300/40 text-[10px] font-semibold leading-none text-rose-100 transition hover:bg-rose-500/20 disabled:cursor-not-allowed disabled:opacity-60"
                        disabled={deleting}
                        onClick={() => {
                          handleRequestDeleteResult(result);
                        }}
                        type="button"
                      >
                        {deleting ? "…" : "X"}
                      </button>
                    </div>

                    <div className="mt-3 flex flex-wrap gap-2 text-xs font-semibold">
                      <span className="rounded-full border border-slate-600 bg-slate-800/80 px-2.5 py-1 text-slate-100">
                        {row.bestActiveEntry.rankDisplay}
                      </span>
                      <span className="rounded-full border border-cyan-300/40 bg-cyan-500/10 px-2.5 py-1 text-cyan-100">
                        {row.bestActiveEntry.formattedPercentage}% match
                      </span>
                      <span className="rounded-full border border-amber-300/40 bg-amber-500/10 px-2.5 py-1 text-amber-100">
                        Target {formatOptionalNumber(row.bestActiveEntry.bestCp)} CP
                      </span>
                    </div>

                    {isDebugMode ? (
                      <dl className="mt-3 grid grid-cols-2 gap-2 border-t border-slate-800 pt-3 text-xs text-slate-300">
                        <div>
                          <dt className="uppercase tracking-wide text-slate-400">Result ID</dt>
                          <dd className="break-all">{result.id}</dd>
                        </div>
                        <div>
                          <dt className="uppercase tracking-wide text-slate-400">Confidence</dt>
                          <dd>{formatConfidence(result.confidence)}</dd>
                        </div>
                        <div>
                          <dt className="uppercase tracking-wide text-slate-400">Created</dt>
                          <dd className="break-all">{result.createdAt}</dd>
                        </div>
                        <div className="col-span-2">
                          <dt className="uppercase tracking-wide text-slate-400">Source</dt>
                          <dd className="break-all">{formatSourceContext(result.source)}</dd>
                        </div>
                      </dl>
                    ) : null}

                    <div className="mt-3">
                      <button
                        aria-controls={`league-details-${result.id}`}
                        aria-expanded={isExpanded}
                        className="min-h-10 w-full rounded-lg border border-slate-600 bg-slate-800/80 px-3 py-2 text-xs font-semibold text-slate-100 transition hover:bg-slate-700"
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
                        }}
                        type="button"
                      >
                        {isExpanded ? `Hide ${selectedLeagueLabel} details` : `Show ${selectedLeagueLabel} details`}
                      </button>

                      {isExpanded ? (
                        <ol className="mt-3 space-y-2" id={`league-details-${result.id}`}>
                          {row.activeLeagueEntries.map((entry) => (
                            <li
                              className="rounded-xl border border-slate-800 bg-slate-900/80 p-3"
                              key={`${result.id}-${entry.evaluatedSpeciesId}-${entry.rank}-${entry.bestCp}`}
                            >
                              <div className="flex flex-wrap items-center justify-between gap-2">
                                <p className="text-sm font-semibold text-slate-100">{entry.speciesDisplayName}</p>
                                <TierChip
                                  ariaLabel={`Tier ${entry.tier} for ${entry.speciesDisplayName}`}
                                  tier={entry.tier}
                                />
                              </div>
                              <dl className="mt-2 grid grid-cols-2 gap-2 text-xs text-slate-200 md:grid-cols-4">
                                <div>
                                  <dt className="uppercase tracking-wide text-slate-400">Target CP</dt>
                                  <dd>{formatOptionalNumber(entry.bestCp)}</dd>
                                </div>
                                <div>
                                  <dt className="uppercase tracking-wide text-slate-400">Match</dt>
                                  <dd>{formatPercentage(entry.percentage)}</dd>
                                </div>
                                <div>
                                  <dt className="uppercase tracking-wide text-slate-400">Rank</dt>
                                  <dd>{isValidRank(entry.rank) ? `#${entry.rank}` : "N/A"}</dd>
                                </div>
                                <div>
                                  <dt className="uppercase tracking-wide text-slate-400">Level</dt>
                                  <dd>{formatOptionalNumber(entry.bestLevel)}</dd>
                                </div>
                              </dl>
                            </li>
                          ))}
                        </ol>
                      ) : null}
                    </div>
                  </article>
                );
              })}
            </section>
          ) : null}

          {phase === allPokemonPhases.SUCCESS && rows.length === 0 ? (
            <p className="mt-4 rounded-lg border border-slate-700 bg-slate-950/80 p-3 text-slate-300">
              No relevant scans available for {selectedLeagueLabel} league.
            </p>
          ) : null}
        </section>

        <div className="flex justify-end pt-1">
          <label className="inline-flex min-h-10 items-center gap-3 rounded-lg border border-slate-700 bg-slate-950 px-3 py-2 text-xs font-semibold text-slate-200">
            <input
              checked={isDebugMode}
              className="h-4 w-4 rounded border-slate-500 bg-slate-900 text-cyan-400 focus:ring-cyan-300"
              onChange={(event) => {
                setIsDebugMode(event.target.checked);
              }}
              type="checkbox"
            />
            Debug mode
          </label>
        </div>
      </div>

      <DeleteResultDialog
        error={pendingDeleteError}
        isDeleting={isDeleteInFlight}
        isOpen={deleteModalState.isOpen}
        onCancel={handleCancelDeleteResult}
        onConfirm={handleConfirmDeleteResult}
        speciesName={deleteModalState.speciesName}
      />
    </main>
  );
}
