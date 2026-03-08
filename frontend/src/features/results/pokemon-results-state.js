export const pokemonResultsPhases = {
  IDLE: "idle",
  LOADING: "loading",
  SUCCESS: "success",
  ERROR: "error",
};

export const initialPokemonResultsState = {
  phase: pokemonResultsPhases.IDLE,
  items: [],
  pendingItems: [],
  lastFetchedAt: "",
  error: null,
};

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
