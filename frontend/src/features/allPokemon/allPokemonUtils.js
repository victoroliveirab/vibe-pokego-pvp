import {
  buildPokemonLeagueBreakdown,
  formatSpeciesDisplayName,
  isValidRank,
  leagueTabs,
  mapMaxCPToLeague,
} from "../results/pokemonLeagueDisplayUtils";

export { isValidRank, leagueTabs, mapMaxCPToLeague };

export const sortOptions = [
  { key: "scanDateAsc", label: "Scan date (oldest first)" },
  { key: "scanDateDesc", label: "Scan date (newest first)" },
  { key: "rankAsc", label: "Rank (best first)" },
  { key: "rankDesc", label: "Rank (worst first)" },
];

function formatRank(rank) {
  if (!isValidRank(rank)) {
    return "N/A";
  }

  return `#${rank}`;
}


export function parseCreatedAt(raw) {
  if (typeof raw !== "string" || raw.trim().length === 0) {
    return Number.NaN;
  }

  const parsed = Date.parse(raw);
  return Number.isNaN(parsed) ? Number.NaN : parsed;
}

export function formatCreatedAt(raw) {
  const parsed = parseCreatedAt(raw);
  if (Number.isNaN(parsed)) {
    return "Unknown";
  }

  return new Date(parsed).toLocaleString();
}

function normalizeDecimal(value) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    return "N/A";
  }

  return value.toFixed(2);
}

export function buildLeagueBreakdown(maxCPEvaluations) {
  return buildPokemonLeagueBreakdown(maxCPEvaluations, {
    transformEntry: (entry) => ({
      ...entry,
      formattedPercentage: normalizeDecimal(entry.percentage),
      rankDisplay: formatRank(entry.rank),
    }),
  }).byLeague;
}

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

export const leagueBestCpThresholds = {
  little: null,
  great: 1400,
  ultra: 2250,
};

export function filterLeagueEntriesByRelevance(entries, league) {
  const normalizedEntries = Array.isArray(entries) ? entries : [];
  const threshold = leagueBestCpThresholds[league] ?? null;

  if (threshold === null) {
    return normalizedEntries;
  }

  return normalizedEntries.filter((entry) => typeof entry?.bestCp === "number" && !Number.isNaN(entry.bestCp) && entry.bestCp >= threshold);
}

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

export function formatSpeciesName(speciesName) {
  return formatSpeciesDisplayName(speciesName);
}
