import {
  buildPokemonLeagueBreakdown,
  formatSpeciesDisplayName,
  isValidRank,
  leagueTabs,
  mapMaxCPToLeague,
} from "../results/pokemonLeagueDisplayUtils";

export { isValidRank, leagueTabs, mapMaxCPToLeague };

/**
 * @typedef {"scanDateAsc"|"scanDateDesc"|"rankAsc"|"rankDesc"} SortOptionKey
 */

/**
 * @typedef {"little"|"great"|"ultra"} LeagueKey
 */

/**
 * @typedef {object} SortOption
 * @property {SortOptionKey} key
 * @property {string} label
 */

/**
 * @typedef {object} PokemonIVs
 * @property {number} attack
 * @property {number} defense
 * @property {number} stamina
 */

/**
 * @typedef {object} PokemonLevel
 * @property {number|null} estimate
 * @property {number|null} confidence
 * @property {string} method
 */

/**
 * @typedef {object} PokemonResultMaxCPEvaluation
 * @property {number} maxCp
 * @property {string} evaluatedSpeciesId
 * @property {number} bestLevel
 * @property {number} bestCp
 * @property {number} statProduct
 * @property {number} rank
 * @property {number} percentage
 */

/**
 * @typedef {PokemonResultMaxCPEvaluation & {
 *   league: LeagueKey,
 *   tier: string,
 *   speciesDisplayName: string,
 *   formattedPercentage?: string,
 *   rankDisplay?: string
 * }} LeagueDisplayEntry
 */

/**
 * @typedef {object} PokemonResultRecord
 * @property {string} id
 * @property {string} speciesName
 * @property {number} cp
 * @property {number} hp
 * @property {PokemonIVs} ivs
 * @property {PokemonLevel} level
 * @property {Array<PokemonResultMaxCPEvaluation>} maxCpEvaluations
 * @property {string} createdAt
 */

/**
 * @typedef {object} LeagueBreakdownBuckets
 * @property {Array<LeagueDisplayEntry>} little
 * @property {Array<LeagueDisplayEntry>} great
 * @property {Array<LeagueDisplayEntry>} ultra
 */

/**
 * @typedef {object} AllPokemonRow
 * @property {Array<LeagueDisplayEntry>} activeLeagueEntries
 * @property {LeagueDisplayEntry} bestActiveEntry
 * @property {LeagueDisplayEntry|null} bestLeagueEntry
 * @property {number|null} bestRank
 * @property {string} bestTier
 * @property {number} scanDate
 * @property {PokemonResultRecord} result
 */

/** @type {Array<SortOption>} */
export const sortOptions = [
  { key: "scanDateAsc", label: "Scan date (oldest first)" },
  { key: "scanDateDesc", label: "Scan date (newest first)" },
  { key: "rankAsc", label: "Rank (best first)" },
  { key: "rankDesc", label: "Rank (worst first)" },
];

/**
 * Formats a numeric rank for display.
 *
 * @param {number} rank
 * @returns {string}
 */
function formatRank(rank) {
  if (!isValidRank(rank)) {
    return "N/A";
  }

  return `#${rank}`;
}

/**
 * Parses a created-at timestamp into epoch milliseconds.
 *
 * @param {string} raw
 * @returns {number}
 */
export function parseCreatedAt(raw) {
  if (typeof raw !== "string" || raw.trim().length === 0) {
    return Number.NaN;
  }

  const parsed = Date.parse(raw);
  return Number.isNaN(parsed) ? Number.NaN : parsed;
}

/**
 * Formats a created-at timestamp for display.
 *
 * @param {string} raw
 * @returns {string}
 */
export function formatCreatedAt(raw) {
  const parsed = parseCreatedAt(raw);
  if (Number.isNaN(parsed)) {
    return "Unknown";
  }

  return new Date(parsed).toLocaleString();
}

/**
 * Formats a numeric value with two decimal places for UI display.
 *
 * @param {number} value
 * @returns {string}
 */
function normalizeDecimal(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return value.toFixed(2);
}

/**
 * Builds a league-indexed breakdown of max-CP evaluations for result display.
 *
 * @param {Array<PokemonResultMaxCPEvaluation>|null|undefined} maxCPEvaluations
 * @returns {LeagueBreakdownBuckets}
 */
export function buildLeagueBreakdown(maxCPEvaluations) {
  return buildPokemonLeagueBreakdown(maxCPEvaluations, {
    transformEntry: (entry) => ({
      ...entry,
      formattedPercentage: normalizeDecimal(entry.percentage),
      rankDisplay: formatRank(entry.rank),
    }),
  }).byLeague;
}

/**
 * Selects the best available league entry across all supported leagues.
 *
 * @param {Partial<LeagueBreakdownBuckets>} byLeague
 * @returns {LeagueDisplayEntry|null}
 */
function selectBestLeagueEntry(byLeague) {
  const allEntries = leagueTabs.flatMap((tab) => (Array.isArray(byLeague[tab.key]) ? byLeague[tab.key] : []));

  if (allEntries.length === 0) {
    return null;
  }

  const orderByLeague = new Map(leagueTabs.map((tab, index) => [tab.key, index]));
  const sortedEntries = [...allEntries].sort((left, right) => {
    const leftPercentage = typeof left?.percentage === "number" && !Number.isNaN(left.percentage) ? left.percentage : Number.NEGATIVE_INFINITY;
    const rightPercentage = typeof right?.percentage === "number" && !Number.isNaN(right.percentage) ? right.percentage : Number.NEGATIVE_INFINITY;

    if (rightPercentage !== leftPercentage) {
      return rightPercentage - leftPercentage;
    }

    const leftRank = isValidRank(left?.rank) ? left.rank : Number.POSITIVE_INFINITY;
    const rightRank = isValidRank(right?.rank) ? right.rank : Number.POSITIVE_INFINITY;

    if (leftRank !== rightRank) {
      return leftRank - rightRank;
    }

    return (orderByLeague.get(left?.league) ?? Number.POSITIVE_INFINITY) - (orderByLeague.get(right?.league) ?? Number.POSITIVE_INFINITY);
  });

  return sortedEntries[0];
}

/** @type {{ little: null, great: number, ultra: number }} */
export const leagueBestCpThresholds = {
  little: null,
  great: 1400,
  ultra: 2250,
};

/**
 * Filters league entries to the subset considered relevant for list views.
 *
 * @param {Array<LeagueDisplayEntry>|null|undefined} entries
 * @param {LeagueKey} league
 * @returns {Array<LeagueDisplayEntry>}
 */
export function filterLeagueEntriesByRelevance(entries, league) {
  const normalizedEntries = Array.isArray(entries) ? entries : [];
  const threshold = leagueBestCpThresholds[league] ?? null;

  if (threshold === null) {
    return normalizedEntries;
  }

  return normalizedEntries.filter((entry) => typeof entry?.bestCp === "number" && !Number.isNaN(entry.bestCp) && entry.bestCp >= threshold);
}

/**
 * Builds the derived row model used by the all-Pokemon listing.
 *
 * @param {PokemonResultRecord} result
 * @param {LeagueKey} league
 * @returns {AllPokemonRow|null}
 */
export function buildAllPokemonRow(result, league) {
  const { byLeague } = buildPokemonLeagueBreakdown(result?.maxCpEvaluations || [], {
    transformEntry: (entry) => ({
      ...entry,
      formattedPercentage: normalizeDecimal(entry.percentage),
      rankDisplay: formatRank(entry.rank),
    }),
  });
  const activeLeagueEntries = filterLeagueEntriesByRelevance(byLeague[league], league);

  if (activeLeagueEntries.length === 0) {
    return null;
  }

  const bestActiveEntry = activeLeagueEntries[0];
  const bestLeagueEntry = selectBestLeagueEntry(byLeague);

  return {
    activeLeagueEntries,
    bestActiveEntry,
    bestLeagueEntry,
    bestRank: isValidRank(bestActiveEntry.rank) ? bestActiveEntry.rank : null,
    bestTier: bestActiveEntry.tier || "N/A",
    scanDate: parseCreatedAt(result?.createdAt),
    result,
  };
}

/**
 * Builds a stable deduplication key from result identity fields.
 *
 * @param {PokemonResultRecord|null|undefined} result
 * @returns {string}
 */
function dedupeIdentity(result) {
  const levelEstimate = result?.level?.estimate;
  const levelPart = typeof levelEstimate === "number" && !Number.isNaN(levelEstimate) ? String(levelEstimate) : "nil";
  const ivs = result?.ivs || {};

  return [
    typeof result?.speciesName === "string" ? result.speciesName.trim().toLowerCase() : "",
    typeof result?.cp === "number" ? result.cp : "",
    typeof result?.hp === "number" ? result.hp : "",
    typeof ivs.attack === "number" ? ivs.attack : "",
    typeof ivs.defense === "number" ? ivs.defense : "",
    typeof ivs.stamina === "number" ? ivs.stamina : "",
    levelPart,
  ].join("|");
}

/**
 * Deduplicates result records by identity, keeping the most recent timestamp.
 *
 * @param {Array<PokemonResultRecord>|null|undefined} results
 * @returns {Array<PokemonResultRecord>}
 */
export function dedupePokemonResults(results) {
  if (!Array.isArray(results)) {
    return [];
  }

  const latestByIdentity = new Map();

  for (const result of results) {
    const key = dedupeIdentity(result);
    const existing = latestByIdentity.get(key);
    const createdAt = parseCreatedAt(result?.createdAt);
    const existingCreatedAt = existing ? existing.createdAtValue : Number.NaN;

    if (!existing || (!Number.isNaN(createdAt) && Number.isNaN(existingCreatedAt)) || createdAt > existingCreatedAt) {
      latestByIdentity.set(key, {
        createdAtValue: Number.isNaN(createdAt) ? existingCreatedAt : createdAt,
        result,
      });
    }
  }

  return Array.from(latestByIdentity.values()).map((entry) => entry.result);
}

/**
 * Formats a species identifier into the shared display name format.
 *
 * @param {string} speciesName
 * @returns {string}
 */
export function formatSpeciesName(speciesName) {
  return formatSpeciesDisplayName(speciesName);
}
