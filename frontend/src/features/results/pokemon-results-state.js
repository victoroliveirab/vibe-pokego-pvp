/**
 * @typedef {object} PokemonResultsIVs
 * @property {number} attack
 * @property {number} defense
 * @property {number} stamina
 */

/**
 * @typedef {object} PokemonResultsLevel
 * @property {number|null} estimate
 * @property {number|null} confidence
 * @property {string} method
 */

/**
 * @typedef {object} PokemonResultsTimeRangeMs
 * @property {number|null} start
 * @property {number|null} end
 */

/**
 * @typedef {object} PokemonResultsSource
 * @property {string} type
 * @property {string} uploadId
 * @property {string} jobId
 * @property {PokemonResultsTimeRangeMs} timeRangeMs
 * @property {number|null} frameTimestampMs
 */

/**
 * @typedef {object} PokemonResultsMaxCPEvaluation
 * @property {number} maxCp
 * @property {string} evaluatedSpeciesId
 * @property {number} bestLevel
 * @property {number} bestCp
 * @property {number} statProduct
 * @property {number} rank
 * @property {number} percentage
 */

/**
 * @typedef {object} PokemonResultItem
 * @property {string} id
 * @property {string} speciesName
 * @property {number} cp
 * @property {number} hp
 * @property {number} powerUpStardustCost
 * @property {PokemonResultsIVs} ivs
 * @property {PokemonResultsLevel} level
 * @property {PokemonResultsSource} source
 * @property {number|null} confidence
 * @property {Array<PokemonResultsMaxCPEvaluation>} maxCpEvaluations
 * @property {string} createdAt
 */

/**
 * @typedef {object} PendingSpeciesOptionItem
 * @property {string} id
 * @property {string} speciesName
 * @property {string} matchMode
 * @property {number} matchDistance
 * @property {number} optionRank
 */

/**
 * @typedef {object} PendingSpeciesSourceItem
 * @property {string} type
 * @property {number|null} frameTimestampMs
 */

/**
 * @typedef {object} PendingSpeciesReadingItem
 * @property {string} id
 * @property {string} jobId
 * @property {string} uploadId
 * @property {number} cp
 * @property {number} hp
 * @property {PokemonResultsIVs} ivs
 * @property {PokemonResultsLevel} level
 * @property {PendingSpeciesSourceItem} source
 * @property {number|null} confidence
 * @property {string} status
 * @property {string} createdAt
 * @property {Array<PendingSpeciesOptionItem>} options
 */

/**
 * @typedef {object} PokemonResultsState
 * @property {"idle"|"loading"|"success"|"error"} phase
 * @property {Array<PokemonResultItem>} items
 * @property {Array<PendingSpeciesReadingItem>} pendingItems
 * @property {string} lastFetchedAt
 * @property {Error|null} error
 */

/**
 * @typedef {object} PokemonResultsStateAction
 * @property {string} type
 * @property {boolean} [preserveItems]
 * @property {Array<PokemonResultItem>} [items]
 * @property {Array<PendingSpeciesReadingItem>} [pendingItems]
 * @property {string} [fetchedAt]
 * @property {Error|null} [error]
 */

/** @type {{ IDLE: "idle", LOADING: "loading", SUCCESS: "success", ERROR: "error" }} */
export const pokemonResultsPhases = {
  IDLE: "idle",
  LOADING: "loading",
  SUCCESS: "success",
  ERROR: "error",
};

/** @type {PokemonResultsState} */
export const initialPokemonResultsState = {
  phase: pokemonResultsPhases.IDLE,
  items: [],
  pendingItems: [],
  lastFetchedAt: "",
  error: null,
};

/**
 * Reduces Pokemon results request lifecycle actions into display state.
 *
 * @param {PokemonResultsState} state
 * @param {PokemonResultsStateAction} action
 * @returns {PokemonResultsState}
 */
export function pokemonResultsStateReducer(state, action) {
  switch (action.type) {
    case "reset":
      return {
        ...initialPokemonResultsState,
      };
    case "request-started":
      return {
        ...state,
        phase: pokemonResultsPhases.LOADING,
        error: null,
        items: action.preserveItems ? state.items : [],
        pendingItems: action.preserveItems ? state.pendingItems : [],
      };
    case "request-succeeded":
      return {
        ...state,
        phase: pokemonResultsPhases.SUCCESS,
        items: Array.isArray(action.items) ? action.items : [],
        pendingItems: Array.isArray(action.pendingItems) ? action.pendingItems : [],
        lastFetchedAt: typeof action.fetchedAt === "string" ? action.fetchedAt : state.lastFetchedAt,
        error: null,
      };
    case "request-failed":
      return {
        ...state,
        phase: pokemonResultsPhases.ERROR,
        error: action.error || null,
      };
    default:
      return state;
  }
}
