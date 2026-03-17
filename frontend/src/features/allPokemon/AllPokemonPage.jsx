import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { getUserFacingErrorMessage } from "../../lib/api-errors";
import { useAnonymousSession } from "../session/useAnonymousSession";
import { createPokemonResultsApi } from "../results/pokemon-results-api";
import { buildLeagueBreakdown, dedupePokemonResults, formatCreatedAt, formatSpeciesName, leagueTabs, parseCreatedAt, sortOptions } from "./allPokemonUtils";

const allPokemonPhases = {
  IDLE: "idle",
  LOADING: "loading",
  SUCCESS: "success",
  ERROR: "error",
};

function normalizePokemonResultsError(error) {
  const code =
    error && typeof error === "object" && typeof error.code === "string" ? error.code : "POKEMON_RESULTS_FAILED";

  return {
    code,
    message: getUserFacingErrorMessage(error),
  };
}

function isValidRank(rank) {
  return typeof rank === "number" && !Number.isNaN(rank) && rank > 0;
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

function mapPokemonToRows(result, league) {
  const byLeague = buildLeagueBreakdown(result.maxCpEvaluations || []);
  const entries = byLeague[league];
  if (!Array.isArray(entries) || entries.length === 0) {
    return null;
  }

  return {
    byLeague,
    bestRank: entries[0] && isValidRank(entries[0].rank) ? entries[0].rank : null,
    scanDate: parseCreatedAt(result.createdAt),
    result,
  };
}

export default function AllPokemonPage({
  pokemonResultsApi: injectedPokemonResultsApi = null,
  useSessionHook = useAnonymousSession,
}) {
  const pokemonResultsApi = useMemo(
    () => injectedPokemonResultsApi || createPokemonResultsApi(),
    [injectedPokemonResultsApi],
  );
  const { sessionId, isLoading: isSessionLoading, error: sessionError } = useSessionHook();
  const [phase, setPhase] = useState(allPokemonPhases.IDLE);
  const [error, setError] = useState(null);
  const [results, setResults] = useState([]);
  const [lastFetchedAt, setLastFetchedAt] = useState("");
  const [selectedLeague, setSelectedLeague] = useState("great");
  const [sortMode, setSortMode] = useState(sortOptions[1].key);
  const requestIdRef = useRef(0);

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
      .map((result) => mapPokemonToRows(result, selectedLeague))
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

  return (
    <main className="mx-auto max-w-5xl p-4 text-sm text-slate-100 md:p-6">
      <section className="rounded-xl border border-slate-800 bg-slate-950/70 p-4">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <h1 className="text-xl font-semibold text-slate-50">All Scanned Pokemon</h1>
            <p className="mt-1 text-xs text-slate-400">
              Accepted appraisal results, deduplicated and grouped by league.
            </p>
          </div>
          <p className="text-xs text-slate-400">{lastFetchedAt ? `Last updated: ${lastFetchedAt}` : "Not synced yet"}</p>
        </div>

        {phase === allPokemonPhases.LOADING && results.length === 0 ? (
          <p className="mt-3 text-cyan-100" role="status">
            Loading scanned Pokemon...
          </p>
        ) : null}

        {!isSessionLoading && sessionError ? (
          <p className="mt-3 rounded-lg border border-rose-500/40 bg-rose-500/10 p-3 text-rose-100" role="alert">
            {getUserFacingErrorMessage(sessionError)}
          </p>
        ) : null}

        <div className="mt-4 flex flex-wrap gap-2">
          {leagueTabs.map((tab) => (
            <button
              key={tab.key}
              aria-pressed={selectedLeague === tab.key}
              className={`min-h-10 rounded-full border px-3 py-2 text-xs font-semibold transition ${
                selectedLeague === tab.key
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

        <label className="mt-4 block text-xs font-semibold uppercase tracking-wide text-slate-300" htmlFor="all-pokemon-sort">
          Sort
          <select
            className="mt-2 min-h-10 w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-xs text-slate-100 outline-none ring-0 focus:border-slate-300 sm:w-auto sm:min-w-72"
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

        {phase === allPokemonPhases.SUCCESS && rows.length === 0 ? (
          <p className="mt-4 rounded-lg border border-slate-700 bg-slate-900/80 p-3 text-slate-300">
            No scans available for {leagueTabs.find((tab) => tab.key === selectedLeague)?.label || "selected"} league.
          </p>
        ) : null}

        {rows.length > 0 ? (
          <section className="mt-4 space-y-3">
            {rows.map((row) => {
              const entries = row.byLeague[selectedLeague];
              const firstEntry = entries[0];
              return (
                <article className="rounded-xl border border-slate-800 bg-slate-900 p-4" data-testid="scan-row" key={row.result.id}>
                  <header className="flex flex-wrap items-center justify-between gap-3">
                    <h2 className="text-sm font-semibold text-slate-100">{formatSpeciesName(row.result.speciesName)}</h2>
                    <span className="text-xs text-slate-400">Scanned: {formatCreatedAt(row.result.createdAt)}</span>
                  </header>
                  <p className="mt-2 text-xs text-slate-200">
                    Best {leagueTabs.find((tab) => tab.key === selectedLeague)?.label || "selected"} rank:{" "}
                    {firstEntry?.rankDisplay || "N/A"} | CP target: {firstEntry?.bestCp || "N/A"} | Match %:{" "}
                    {firstEntry?.formattedPercentage || "N/A"}
                  </p>
                  <dl className="mt-2 grid grid-cols-2 gap-2 text-xs text-slate-300 sm:grid-cols-3">
                    <div>
                      <dt className="text-slate-500 uppercase tracking-wide">CP / HP</dt>
                      <dd>
                        {row.result.cp} / {row.result.hp}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-slate-500 uppercase tracking-wide">IVs</dt>
                      <dd>
                        {`${row.result.ivs?.attack ?? "N/A"}/${row.result.ivs?.defense ?? "N/A"}/${row.result.ivs?.stamina ?? "N/A"}`}
                      </dd>
                    </div>
                    <div>
                      <dt className="text-slate-500 uppercase tracking-wide">Scan source</dt>
                      <dd>{row.result.source?.type || "Unknown"}</dd>
                    </div>
                  </dl>

                  <ul className="mt-3 space-y-1 text-xs text-slate-200">
                    {entries.map((entry) => (
                      <li className="rounded-lg border border-slate-700/70 bg-slate-950/80 px-2 py-1.5" key={`${row.result.id}-${entry.evaluatedSpeciesId}-${entry.rank}-${entry.maxCp}`}>
                        <span className="font-semibold text-slate-100">{entry.speciesDisplayName}</span>
                        <span className="ml-2 text-slate-400">
                          {entry.maxCp} CP, {entry.rankDisplay}, {entry.formattedPercentage}%
                        </span>
                      </li>
                    ))}
                  </ul>
                </article>
              );
            })}
          </section>
        ) : null}
      </section>
    </main>
  );
}
