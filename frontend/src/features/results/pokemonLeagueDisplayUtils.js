export const leagueTabs = [
  { key: "little", label: "Little" },
  { key: "great", label: "Great" },
  { key: "ultra", label: "Ultra" },
];

function isValidPercentage(value) {
  return typeof value === "number" && !Number.isNaN(value);
}

export function normalizeMaxCPEvaluations(maxCPEvaluations) {
  return Array.isArray(maxCPEvaluations) ? maxCPEvaluations : [];
}

export function isValidRank(rank) {
  return typeof rank === "number" && !Number.isNaN(rank) && rank > 0;
}

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
