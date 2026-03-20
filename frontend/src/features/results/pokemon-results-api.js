import { createApiClient } from "../../lib/api-client";
import { APIClientError } from "../../lib/api-errors";

const defaultApiClient = createApiClient();

/**
 * @typedef {null|boolean|number|string|JsonObject|JsonArray} JsonValue
 */

/**
 * @typedef {Object.<string, JsonValue>} JsonObject
 */

/**
 * @typedef {Array<JsonValue>} JsonArray
 */

/**
 * @typedef {object} PokemonResultsApiClient
 * @property {function(string, { method?: string, headers?: HeadersInit, body?: BodyInit|null, requiresSession?: boolean, sessionId?: string }=): Promise<JsonValue|null>} request
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
 * @typedef {object} PokemonTimeRangeMs
 * @property {number|null} start
 * @property {number|null} end
 */

/**
 * @typedef {object} PokemonSource
 * @property {string} type
 * @property {string} uploadId
 * @property {string} jobId
 * @property {PokemonTimeRangeMs} timeRangeMs
 * @property {number|null} frameTimestampMs
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
 * @typedef {object} PokemonResult
 * @property {string} id
 * @property {string} speciesName
 * @property {number} cp
 * @property {number} hp
 * @property {number} powerUpStardustCost
 * @property {PokemonIVs} ivs
 * @property {PokemonLevel} level
 * @property {PokemonSource} source
 * @property {number|null} confidence
 * @property {Array<MaxCPEvaluation>} maxCpEvaluations
 * @property {string} createdAt
 */

/**
 * @typedef {object} PokemonResultsResponse
 * @property {Array<PokemonResult>} results
 */

/**
 * @typedef {object} PendingSpeciesOption
 * @property {string} id
 * @property {string} speciesName
 * @property {string} matchMode
 * @property {number} matchDistance
 * @property {number} optionRank
 */

/**
 * @typedef {object} PendingSpeciesSource
 * @property {string} type
 * @property {number|null} frameTimestampMs
 */

/**
 * @typedef {object} PendingSpeciesReading
 * @property {string} id
 * @property {string} jobId
 * @property {string} uploadId
 * @property {number} cp
 * @property {number} hp
 * @property {PokemonIVs} ivs
 * @property {PokemonLevel} level
 * @property {PendingSpeciesSource} source
 * @property {number|null} confidence
 * @property {string} status
 * @property {string} createdAt
 * @property {Array<PendingSpeciesOption>} options
 */

/**
 * @typedef {object} PendingSpeciesReadingsResponse
 * @property {Array<PendingSpeciesReading>} readings
 */

/**
 * @typedef {object} ResolvePendingSpeciesResponse
 * @property {PokemonResult} result
 */

/**
 * @typedef {object} PokemonResultsRequestOptions
 * @property {string} [sessionId=""]
 */

/**
 * @typedef {object} ResolvePendingSpeciesReadingOptions
 * @property {string} [sessionId=""]
 * @property {string} [readingId=""]
 * @property {string} [optionId=""]
 */

/**
 * @typedef {object} DeletePokemonResultOptions
 * @property {string} [sessionId=""]
 * @property {string} [resultId=""]
 */

/**
 * @typedef {object} PokemonResultsApi
 * @property {function(PokemonResultsRequestOptions=): Promise<PokemonResultsResponse>} getPokemonResults
 * @property {function(PokemonResultsRequestOptions=): Promise<PendingSpeciesReadingsResponse>} getPendingSpeciesReadings
 * @property {function(ResolvePendingSpeciesReadingOptions=): Promise<ResolvePendingSpeciesResponse>} resolvePendingSpeciesReading
 * @property {function(DeletePokemonResultOptions=): Promise<void>} deletePokemonResult
 */

/**
 * @typedef {object} CreatePokemonResultsApiOptions
 * @property {PokemonResultsApiClient} [apiClient]
 */

/**
 * Creates a normalized invalid-response error for the Pokemon results API.
 *
 * @param {string} message
 * @param {JsonValue|null} details
 * @returns {APIClientError}
 */
function invalidResponse(message, details) {
  return new APIClientError({
    code: "INVALID_RESPONSE",
    message,
    details,
  });
}

/**
 * Normalizes a required non-empty string field from a response payload.
 *
 * @param {unknown} value
 * @param {string} fieldName
 * @param {JsonValue|null} payload
 * @returns {string}
 * @throws {APIClientError}
 */
function normalizeRequiredString(value, fieldName, payload) {
  if (typeof value !== "string" || value.trim().length === 0) {
    throw invalidResponse(`Pokemon results response was missing ${fieldName}.`, payload);
  }

  return value;
}

/**
 * Normalizes a required numeric field from a response payload.
 *
 * @param {unknown} value
 * @param {string} fieldName
 * @param {JsonValue|null} payload
 * @returns {number}
 * @throws {APIClientError}
 */
function normalizeRequiredNumber(value, fieldName, payload) {
  if (typeof value !== "number" || Number.isNaN(value)) {
    throw invalidResponse(`Pokemon results response was missing ${fieldName}.`, payload);
  }

  return value;
}

/**
 * Normalizes a nullable numeric field from a response payload.
 *
 * @param {unknown} value
 * @param {string} fieldName
 * @param {JsonValue|null} payload
 * @returns {number|null}
 * @throws {APIClientError}
 */
function normalizeOptionalNullableNumber(value, fieldName, payload) {
  if (value === null || value === undefined) {
    return null;
  }

  return normalizeRequiredNumber(value, fieldName, payload);
}

/**
 * Normalizes the IV payload attached to a Pokemon result.
 *
 * @param {JsonObject|null} ivsValue
 * @param {JsonValue|null} payload
 * @returns {PokemonIVs}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes the level payload attached to a Pokemon result.
 *
 * @param {JsonObject|null} levelValue
 * @param {JsonValue|null} payload
 * @returns {PokemonLevel}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes the optional video time range metadata for a result source.
 *
 * @param {JsonObject|null} timeRangeMsValue
 * @param {JsonValue|null} payload
 * @returns {PokemonTimeRangeMs}
 * @throws {APIClientError}
 */
function normalizeTimeRangeMs(timeRangeMsValue, payload) {
  if (!timeRangeMsValue || typeof timeRangeMsValue !== "object") {
    throw invalidResponse("Pokemon results response had an invalid source.timeRangeMs object.", payload);
  }

  return {
    start: normalizeOptionalNullableNumber(timeRangeMsValue.start, "source.timeRangeMs.start", payload),
    end: normalizeOptionalNullableNumber(timeRangeMsValue.end, "source.timeRangeMs.end", payload),
  };
}

/**
 * Normalizes the source metadata for a resolved Pokemon result.
 *
 * @param {JsonObject|null} sourceValue
 * @param {JsonValue|null} payload
 * @returns {PokemonSource}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes a single Pokemon result record.
 *
 * @param {JsonObject|null} record
 * @returns {PokemonResult}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes one max-CP evaluation entry for PvP calculations.
 *
 * @param {JsonObject|null} value
 * @param {JsonValue|null} payload
 * @returns {MaxCPEvaluation}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes the max-CP evaluations array for a result record.
 *
 * @param {JsonArray|null|undefined} value
 * @param {JsonValue|null} payload
 * @returns {Array<MaxCPEvaluation>}
 * @throws {APIClientError}
 */
function normalizeMaxCPEvaluations(value, payload) {
  if (value === null || value === undefined) {
    return [];
  }

  if (!Array.isArray(value)) {
    throw invalidResponse("Pokemon results response had an invalid maxCpEvaluations array.", payload);
  }

  return value.map((entry) => normalizeMaxCPEvaluation(entry, payload));
}

/**
 * Normalizes the top-level Pokemon results payload.
 *
 * @param {JsonObject|null} payload
 * @returns {PokemonResultsResponse}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes one pending-species option record.
 *
 * @param {JsonObject|null} optionValue
 * @param {JsonValue|null} payload
 * @returns {PendingSpeciesOption}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes the source metadata for a pending species reading.
 *
 * @param {JsonObject|null} sourceValue
 * @param {JsonValue|null} payload
 * @returns {PendingSpeciesSource}
 * @throws {APIClientError}
 */
function normalizePendingSource(sourceValue, payload) {
  if (!sourceValue || typeof sourceValue !== "object") {
    throw invalidResponse("Pending species response had an invalid source object.", payload);
  }

  return {
    type: normalizeRequiredString(sourceValue.type, "source.type", payload),
    frameTimestampMs: normalizeOptionalNullableNumber(sourceValue.frameTimestampMs, "source.frameTimestampMs", payload),
  };
}

/**
 * Normalizes one pending species reading record.
 *
 * @param {JsonObject|null} record
 * @returns {PendingSpeciesReading}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes the top-level pending-species readings response payload.
 *
 * @param {JsonObject|null} payload
 * @returns {PendingSpeciesReadingsResponse}
 * @throws {APIClientError}
 */
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

/**
 * Normalizes the response returned after resolving a pending species reading.
 *
 * @param {JsonObject|null} payload
 * @returns {ResolvePendingSpeciesResponse}
 * @throws {APIClientError}
 */
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

/**
 * Creates the Pokemon results API facade used by results-oriented screens.
 *
 * @param {CreatePokemonResultsApiOptions} [options={}]
 * @returns {PokemonResultsApi}
 */
export function createPokemonResultsApi({ apiClient = defaultApiClient } = {}) {
  return {
    /**
     * Fetches and normalizes the resolved Pokemon results for the current session.
     *
     * @param {PokemonResultsRequestOptions} [options={}]
     * @returns {Promise<PokemonResultsResponse>}
     * @throws {APIClientError}
     */
    async getPokemonResults({ sessionId = "" } = {}) {
      const payload = await apiClient.request("/pokemon", {
        method: "GET",
        requiresSession: true,
        sessionId,
      });

      return normalizeResultsResponse(payload);
    },
    /**
     * Fetches and normalizes pending species readings that still need resolution.
     *
     * @param {PokemonResultsRequestOptions} [options={}]
     * @returns {Promise<PendingSpeciesReadingsResponse>}
     * @throws {APIClientError}
     */
    async getPendingSpeciesReadings({ sessionId = "" } = {}) {
      const payload = await apiClient.request("/pokemon/pending-species", {
        method: "GET",
        requiresSession: true,
        sessionId,
      });

      return normalizePendingReadingsResponse(payload);
    },
    /**
     * Resolves a pending species reading using a selected option.
     *
     * @param {ResolvePendingSpeciesReadingOptions} [options={}]
     * @returns {Promise<ResolvePendingSpeciesResponse>}
     * @throws {APIClientError}
     */
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
    /**
     * Deletes a normalized Pokemon result for the current session.
     *
     * @param {DeletePokemonResultOptions} [options={}]
     * @returns {Promise<void>}
     * @throws {APIClientError}
     */
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

/**
 * Convenience wrapper for fetching normalized Pokemon results with the default API client.
 *
 * @param {PokemonResultsRequestOptions} [options={}]
 * @returns {Promise<PokemonResultsResponse>}
 */
export async function getPokemonResults({ sessionId = "" } = {}) {
  return defaultPokemonResultsApi.getPokemonResults({ sessionId });
}
