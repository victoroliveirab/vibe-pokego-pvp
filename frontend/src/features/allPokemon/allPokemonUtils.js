export const leagueTabs = [
  { key: "little", label: "Little" },
  { key: "great", label: "Great" },
  { key: "ultra", label: "Ultra" },
];

export const sortOptions = [
  { key: "scanDateAsc", label: "Scan date (oldest first)" },
  { key: "scanDateDesc", label: "Scan date (newest first)" },
  { key: "rankAsc", label: "Rank (best first)" },
  { key: "rankDesc", label: "Rank (worst first)" },
];

function isValidRank(rank) {
  return typeof rank === "number" && !Number.isNaN(rank) && rank > 0;
}

function formatRank(rank) {
  if (!isValidRank(rank)) {
    return "N/A";
  }

  return `#${rank}`;
}

function isValidPercentage(value) {
  return typeof value === "number" && !Number.isNaN(value);
}

function compareLeagueEntries(left, right) {
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

function normalizeSpeciesDisplayName(speciesId) {
  if (typeof speciesId !== "string" || speciesId.trim().length === 0) {
    return "Unknown";
  }

  return speciesId
    .trim()
    .replace(/[\-_]+/g, " ")
    .replace(/\s+/g, " ")
    .replace(/\b\w/g, (character) => character.toUpperCase());
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
  const byLeague = {
    little: [],
    great: [],
    ultra: [],
  };

  const entries = Array.isArray(maxCPEvaluations) ? maxCPEvaluations : [];

  for (const entry of entries) {
    if (!entry || typeof entry !== "object") {
      continue;
    }

    const league = mapMaxCPToLeague(entry.maxCp);
    if (!league) {
      continue;
    }

    byLeague[league].push({
      ...entry,
      league,
      speciesDisplayName: normalizeSpeciesDisplayName(entry.evaluatedSpeciesId),
      formattedPercentage: normalizeDecimal(entry.percentage),
      formattedBestCp: isValidRank(entry.bestCp) ? entry.bestCp : entry.bestCp,
      rankDisplay: formatRank(entry.rank),
    });
  }

  for (const key of leagueTabs.map((tab) => tab.key)) {
    byLeague[key].sort(compareLeagueEntries);
  }

  return byLeague;
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
  return normalizeSpeciesDisplayName(speciesName);
}
