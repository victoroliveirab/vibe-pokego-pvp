import { createApiClient } from "../../lib/api-client";
import { APIClientError } from "../../lib/api-errors";

const defaultApiClient = createApiClient();

function invalidResponse(message, details) {
  return new APIClientError({
    code: "INVALID_RESPONSE",
    message,
    details,
  });
}

function normalizeRequiredString(value, fieldName, payload) {
  if (typeof value !== "string" || value.trim().length === 0) {
    throw invalidResponse(`Pokemon results response was missing ${fieldName}.`, payload);
  }

  return value;
}

function normalizeRequiredNumber(value, fieldName, payload) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    throw invalidResponse(`Pokemon results response was missing ${fieldName}.`, payload);
  }

  return value;
}

function normalizeOptionalNullableNumber(value, fieldName, payload) {
  if (value === null || value === undefined) {
    return null;
  }

  return normalizeRequiredNumber(value, fieldName, payload);
}

function normalizeIVs(ivsValue, payload) {
  if (!ivsValue || typeof ivsValue !== "object") {
    throw invalidResponse("Pokemon results response had an invalid ivs object.", payload);
  }

  return {
    attack: normalizeRequiredNumber(ivsValue.attack, "ivs.attack", payload),
    defense: normalizeRequiredNumber(ivsValue.defense, "ivs.defense", payload),
    stamina: normalizeRequiredNumber(ivsValue.stamina, "ivs.stamina", payload),
  };
}

function normalizeLevel(levelValue, payload) {
  if (!levelValue || typeof levelValue !== "object") {
    throw invalidResponse("Pokemon results response had an invalid level object.", payload);
  }

  return {
    estimate: normalizeOptionalNullableNumber(levelValue.estimate, "level.estimate", payload),
    confidence: normalizeOptionalNullableNumber(levelValue.confidence, "level.confidence", payload),
    method: normalizeRequiredString(levelValue.method, "level.method", payload),
  };
}

function normalizeTimeRangeMs(timeRangeMsValue, payload) {
  if (!timeRangeMsValue || typeof timeRangeMsValue !== "object") {
    throw invalidResponse("Pokemon results response had an invalid source.timeRangeMs object.", payload);
  }

  return {
    start: normalizeOptionalNullableNumber(timeRangeMsValue.start, "source.timeRangeMs.start", payload),
    end: normalizeOptionalNullableNumber(timeRangeMsValue.end, "source.timeRangeMs.end", payload),
  };
}

function normalizeSource(sourceValue, payload) {
  if (!sourceValue || typeof sourceValue !== "object") {
    throw invalidResponse("Pokemon results response had an invalid source object.", payload);
  }

  return {
    type: normalizeRequiredString(sourceValue.type, "source.type", payload),
    uploadId: normalizeRequiredString(sourceValue.uploadId, "source.uploadId", payload),
    jobId: normalizeRequiredString(sourceValue.jobId, "source.jobId", payload),
    timeRangeMs: normalizeTimeRangeMs(sourceValue.timeRangeMs, payload),
    frameTimestampMs: normalizeOptionalNullableNumber(sourceValue.frameTimestampMs, "source.frameTimestampMs", payload),
  };
}

function normalizeResultRecord(record) {
  if (!record || typeof record !== "object") {
    throw invalidResponse("Pokemon results response had an invalid result item.", record);
  }

  return {
    id: normalizeRequiredString(record.id, "id", record),
    speciesName: normalizeRequiredString(record.speciesName, "speciesName", record),
    cp: normalizeRequiredNumber(record.cp, "cp", record),
    hp: normalizeRequiredNumber(record.hp, "hp", record),
    powerUpStardustCost: normalizeRequiredNumber(record.powerUpStardustCost, "powerUpStardustCost", record),
    ivs: normalizeIVs(record.ivs, record),
    level: normalizeLevel(record.level, record),
    source: normalizeSource(record.source, record),
    confidence: normalizeOptionalNullableNumber(record.confidence, "confidence", record),
    maxCpEvaluations: normalizeMaxCPEvaluations(record.maxCpEvaluations, record),
    createdAt: normalizeRequiredString(record.createdAt, "createdAt", record),
  };
}

function normalizeMaxCPEvaluation(value, payload) {
  if (!value || typeof value !== "object") {
    throw invalidResponse("Pokemon results response had an invalid maxCpEvaluations item.", payload);
  }

  return {
    maxCp: normalizeRequiredNumber(value.maxCp, "maxCpEvaluations.maxCp", payload),
    evaluatedSpeciesId: normalizeRequiredString(
      value.evaluatedSpeciesId,
      "maxCpEvaluations.evaluatedSpeciesId",
      payload,
    ),
    bestLevel: normalizeRequiredNumber(value.bestLevel, "maxCpEvaluations.bestLevel", payload),
    bestCp: normalizeRequiredNumber(value.bestCp, "maxCpEvaluations.bestCp", payload),
    statProduct: normalizeRequiredNumber(value.statProduct, "maxCpEvaluations.statProduct", payload),
    rank: normalizeRequiredNumber(value.rank, "maxCpEvaluations.rank", payload),
    percentage: normalizeRequiredNumber(value.percentage, "maxCpEvaluations.percentage", payload),
  };
}

function normalizeMaxCPEvaluations(value, payload) {
  if (value === null || value === undefined) {
    return [];
  }

  if (!Array.isArray(value)) {
    throw invalidResponse("Pokemon results response had an invalid maxCpEvaluations array.", payload);
  }

  return value.map((entry) => normalizeMaxCPEvaluation(entry, payload));
}

function normalizeResultsResponse(payload) {
  if (!payload || typeof payload !== "object") {
    throw invalidResponse("Pokemon results response payload was invalid.", payload);
  }

  if (!Array.isArray(payload.results)) {
    throw invalidResponse("Pokemon results response was missing results array.", payload);
  }

  return {
    results: payload.results.map((record) => normalizeResultRecord(record)),
  };
}

function normalizePendingOption(optionValue, payload) {
  if (!optionValue || typeof optionValue !== "object") {
    throw invalidResponse("Pending species response had an invalid option item.", payload);
  }

  return {
    id: normalizeRequiredString(optionValue.id, "options.id", payload),
    speciesName: normalizeRequiredString(optionValue.speciesName, "options.speciesName", payload),
    matchMode: normalizeRequiredString(optionValue.matchMode, "options.matchMode", payload),
    matchDistance: normalizeRequiredNumber(optionValue.matchDistance, "options.matchDistance", payload),
    optionRank: normalizeRequiredNumber(optionValue.optionRank, "options.optionRank", payload),
  };
}

function normalizePendingSource(sourceValue, payload) {
  if (!sourceValue || typeof sourceValue !== "object") {
    throw invalidResponse("Pending species response had an invalid source object.", payload);
  }

  return {
    type: normalizeRequiredString(sourceValue.type, "source.type", payload),
    frameTimestampMs: normalizeOptionalNullableNumber(sourceValue.frameTimestampMs, "source.frameTimestampMs", payload),
  };
}

function normalizePendingReading(record) {
  if (!record || typeof record !== "object") {
    throw invalidResponse("Pending species response had an invalid reading item.", record);
  }

  if (!Array.isArray(record.options)) {
    throw invalidResponse("Pending species reading was missing options array.", record);
  }

  return {
    id: normalizeRequiredString(record.id, "id", record),
    jobId: normalizeRequiredString(record.jobId, "jobId", record),
    uploadId: normalizeRequiredString(record.uploadId, "uploadId", record),
    cp: normalizeRequiredNumber(record.cp, "cp", record),
    hp: normalizeRequiredNumber(record.hp, "hp", record),
    ivs: normalizeIVs(record.ivs, record),
    level: normalizeLevel(record.level, record),
    source: normalizePendingSource(record.source, record),
    confidence: normalizeOptionalNullableNumber(record.confidence, "confidence", record),
    status: normalizeRequiredString(record.status, "status", record),
    createdAt: normalizeRequiredString(record.createdAt, "createdAt", record),
    options: record.options.map((option) => normalizePendingOption(option, record)),
  };
}

function normalizePendingReadingsResponse(payload) {
  if (!payload || typeof payload !== "object") {
    throw invalidResponse("Pending species response payload was invalid.", payload);
  }

  if (!Array.isArray(payload.readings)) {
    throw invalidResponse("Pending species response was missing readings array.", payload);
  }

  return {
    readings: payload.readings.map((record) => normalizePendingReading(record)),
  };
}

function normalizeResolveResponse(payload) {
  if (!payload || typeof payload !== "object") {
    throw invalidResponse("Pending species resolve response payload was invalid.", payload);
  }
  if (!payload.result || typeof payload.result !== "object") {
    throw invalidResponse("Pending species resolve response was missing result object.", payload);
  }

  return {
    result: normalizeResultRecord(payload.result),
  };
}

export function createPokemonResultsApi({ apiClient = defaultApiClient } = {}) {
  return {
    async getPokemonResults({ sessionId = "" } = {}) {
      const payload = await apiClient.request("/pokemon", {
        method: "GET",
        requiresSession: true,
        sessionId,
      });

      return normalizeResultsResponse(payload);
    },
    async getPendingSpeciesReadings({ sessionId = "" } = {}) {
      const payload = await apiClient.request("/pokemon/pending-species", {
        method: "GET",
        requiresSession: true,
        sessionId,
      });

      return normalizePendingReadingsResponse(payload);
    },
    async resolvePendingSpeciesReading({ sessionId = "", readingId = "", optionId = "" } = {}) {
      const normalizedReadingID = normalizeRequiredString(readingId, "readingId", { readingId, optionId });
      const normalizedOptionID = normalizeRequiredString(optionId, "optionId", { readingId, optionId });
      const payload = await apiClient.request(`/pokemon/pending-species/${encodeURIComponent(normalizedReadingID)}`, {
        method: "PATCH",
        requiresSession: true,
        sessionId,
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          optionId: normalizedOptionID,
        }),
      });

      return normalizeResolveResponse(payload);
    },
    async deletePokemonResult({ sessionId = "", resultId = "" } = {}) {
      const normalizedResultID = normalizeRequiredString(resultId, "resultId", { resultId });
      await apiClient.request(`/pokemon/${encodeURIComponent(normalizedResultID)}`, {
        method: "DELETE",
        requiresSession: true,
        sessionId,
      });
    },
  };
}

const defaultPokemonResultsApi = createPokemonResultsApi();

export async function getPokemonResults({ sessionId = "" } = {}) {
  return defaultPokemonResultsApi.getPokemonResults({ sessionId });
}
