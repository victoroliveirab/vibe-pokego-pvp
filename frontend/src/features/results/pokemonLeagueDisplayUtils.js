/**
 * @typedef {"little"|"great"|"ultra"} LeagueKey
 */

/**
 * @typedef {"S"|"A"|"B"|"C"|"D"|"F"} LeagueTier
 */

/**
 * @typedef {object} LeagueTab
 * @property {LeagueKey} key
 * @property {string} label
 */

/**
 * @typedef {object} MaxCPEvaluation
 * @property {number} maxCp
 * @property {string} evaluatedSpeciesId
 * @property {number} bestLevel
 * @property {number} bestCp
 * @property {number} statProduct
 * @property {number} rank
 * @property {number} percentage
 */

/**
 * @typedef {MaxCPEvaluation & {
 *   league: LeagueKey,
 *   tier: LeagueTier,
 *   speciesDisplayName: string
 * }} LeagueEntry
 */

/**
 * @typedef {LeagueEntry & {
 *   formattedPercentage?: string,
 *   rankDisplay?: string
 * }} LeagueDisplayEntry
 */

/**
 * @typedef {object} LeagueBreakdownBuckets
 * @property {Array<LeagueEntry|LeagueDisplayEntry>} little
 * @property {Array<LeagueEntry|LeagueDisplayEntry>} great
 * @property {Array<LeagueEntry|LeagueDisplayEntry>} ultra
 */

/**
 * @typedef {object} PokemonLeagueBreakdown
 * @property {LeagueBreakdownBuckets} byLeague
 * @property {LeagueTier|null} bestAvailableTier
 */

/** @type {Array<LeagueTab>} */
export const leagueTabs = [
  { key: "little", label: "Little" },
  { key: "great", label: "Great" },
  { key: "ultra", label: "Ultra" },
];

/**
 * Determines whether a percentage value can participate in ranking comparisons.
 *
 * @param {number} value
 * @returns {boolean}
 */
function isValidPercentage(value) {
  return typeof value === "number" && !Number.isNaN(value);
}

/**
 * Normalizes a max-CP evaluations collection into an array.
 *
 * @param {Array<MaxCPEvaluation>|null|undefined} maxCPEvaluations
 * @returns {Array<MaxCPEvaluation>}
 */
export function normalizeMaxCPEvaluations(maxCPEvaluations) {
  return Array.isArray(maxCPEvaluations) ? maxCPEvaluations : [];
}

/**
 * Checks whether a rank value is a positive numeric rank.
 *
 * @param {number} rank
 * @returns {boolean}
 */
export function isValidRank(rank) {
  return typeof rank === "number" && !Number.isNaN(rank) && rank > 0;
}

/**
 * Maps a PvP rank to the tier chip used in the UI.
 *
 * @param {number} rank
 * @returns {LeagueTier}
 */
export function rankToTier(rank) {
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

/**
 * Returns the Tailwind classes for a league tier badge.
 *
 * @param {LeagueTier|string} tier
 * @returns {string}
 */
export function tierChipClasses(tier) {
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

/**
 * Maps a Pokemon GO PvP CP cap to the corresponding league key.
 *
 * @param {number} maxCp
 * @returns {LeagueKey|null}
 */
export function mapMaxCPToLeague(maxCp) {
  if (maxCp === 500) {
    return "little";
  }
  if (maxCp === 1500) {
    return "great";
  }
  if (maxCp === 2500) {
    return "ultra";
  }
  return null;
}

/**
 * Formats a species identifier into a human-readable display name.
 *
 * @param {string} speciesId
 * @returns {string}
 */
export function formatSpeciesDisplayName(speciesId) {
  if (typeof speciesId !== "string" || speciesId.trim().length === 0) {
    return "Unknown";
  }

  return speciesId
    .trim()
    .replace(/[\-_]+/g, " ")
    .replace(/\s+/g, " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
}

/**
 * Sorts league entries by rank, then percentage, then max CP, then species ID.
 *
 * @param {LeagueEntry} left
 * @param {LeagueEntry} right
 * @returns {number}
 */
export function compareLeagueEntries(left, right) {
  const leftRank = isValidRank(left.rank) ? left.rank : Number.POSITIVE_INFINITY;
  const rightRank = isValidRank(right.rank) ? right.rank : Number.POSITIVE_INFINITY;

  if (leftRank !== rightRank) {
    return leftRank - rightRank;
  }

  const leftPercentage = isValidPercentage(left.percentage) ? left.percentage : Number.NEGATIVE_INFINITY;
  const rightPercentage = isValidPercentage(right.percentage) ? right.percentage : Number.NEGATIVE_INFINITY;

  if (rightPercentage !== leftPercentage) {
    return rightPercentage - leftPercentage;
  }

  if (left.maxCp !== right.maxCp) {
    return right.maxCp - left.maxCp;
  }

  return String(left.evaluatedSpeciesId).localeCompare(String(right.evaluatedSpeciesId));
}

/**
 * Groups max-CP evaluations by league and enriches them for UI display.
 *
 * @param {Array<MaxCPEvaluation>|null|undefined} maxCPEvaluations
 * @param {{ transformEntry?: function(LeagueEntry): (LeagueEntry|LeagueDisplayEntry) }} [options={}]
 * @returns {PokemonLeagueBreakdown}
 */
export function buildPokemonLeagueBreakdown(maxCPEvaluations, { transformEntry } = {}) {
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

    const baseEntry = {
      ...entry,
      league,
      tier: rankToTier(entry.rank),
      speciesDisplayName: formatSpeciesDisplayName(entry.evaluatedSpeciesId),
    };
    const nextEntry = typeof transformEntry === "function" ? transformEntry(baseEntry) : baseEntry;

    byLeague[league].push(nextEntry);

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
