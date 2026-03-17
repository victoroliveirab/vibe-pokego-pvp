import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import AllPokemonPage from "./AllPokemonPage";

function createSessionHook() {
  return () => ({
    sessionId: "session-1",
    isLoading: false,
    error: null,
  });
}

function createPokemonResultsApi(payload = []) {
  return {
    getPokemonResults: vi.fn().mockResolvedValue({
      results: payload,
    }),
  };
}

describe("all pokemon page", () => {
  it("filters rows by selected league and shows all options for that league", async () => {
    const pokemonResultsApi = createPokemonResultsApi([
      {
        id: "result-1",
        speciesName: "Bulbasaur",
        cp: 300,
        hp: 90,
        ivs: { attack: 12, defense: 12, stamina: 12 },
        level: { estimate: 10, confidence: 0.7, method: "ARC_POSITION" },
        source: { type: "IMAGE" },
        maxCpEvaluations: [
          {
            maxCp: 500,
            evaluatedSpeciesId: "bulba-little",
            bestLevel: 10,
            bestCp: 500,
            statProduct: 1.2,
            rank: 50,
            percentage: 92.1,
          },
          {
            maxCp: 1500,
            evaluatedSpeciesId: "bulba-great",
            bestLevel: 20,
            bestCp: 1490,
            statProduct: 2,
            rank: 25,
            percentage: 83.0,
          },
        ],
        createdAt: "2026-03-05T20:00:00Z",
      },
      {
        id: "result-2",
        speciesName: "Charmander",
        cp: 350,
        hp: 70,
        ivs: { attack: 10, defense: 11, stamina: 10 },
        level: { estimate: 11, confidence: 0.6, method: "ARC_POSITION" },
        source: { type: "VIDEO" },
        maxCpEvaluations: [
          {
            maxCp: 2500,
            evaluatedSpeciesId: "char-ultra",
            bestLevel: 40,
            bestCp: 2488,
            statProduct: 3.3,
            rank: 14,
            percentage: 91.3,
          },
        ],
        createdAt: "2026-03-05T20:20:00Z",
      },
    ]);

    render(<AllPokemonPage pokemonResultsApi={pokemonResultsApi} useSessionHook={createSessionHook()} />);

    await waitFor(() => {
      expect(screen.getByText("Bulbasaur")).toBeTruthy();
    });

    expect(screen.queryByText("Charmander")).toBeNull();

    const row = screen.getByText("Bulbasaur").closest("[data-testid='scan-row']");
    expect(row).toBeTruthy();
    expect(within(row).getByText(/Bulba Great/i)).toBeTruthy();
  });

  it("sorts rows with a single combined sort control", async () => {
    const pokemonResultsApi = createPokemonResultsApi([
      {
        id: "result-1",
        speciesName: "Zubat",
        cp: 100,
        hp: 60,
        ivs: { attack: 10, defense: 10, stamina: 10 },
        level: { estimate: 10, confidence: 0.5, method: "ARC_POSITION" },
        source: { type: "IMAGE" },
        maxCpEvaluations: [
          {
            maxCp: 1500,
            evaluatedSpeciesId: "zubat",
            bestLevel: 20,
            bestCp: 1490,
            statProduct: 2.2,
            rank: 40,
            percentage: 85.1,
          },
        ],
        createdAt: "2026-03-05T20:10:00Z",
      },
      {
        id: "result-2",
        speciesName: "Aipom",
        cp: 120,
        hp: 65,
        ivs: { attack: 10, defense: 9, stamina: 11 },
        level: { estimate: 10, confidence: 0.5, method: "ARC_POSITION" },
        source: { type: "IMAGE" },
        maxCpEvaluations: [
          {
            maxCp: 1500,
            evaluatedSpeciesId: "aipom",
            bestLevel: 20,
            bestCp: 1490,
            statProduct: 2.2,
            rank: 20,
            percentage: 84.8,
          },
        ],
        createdAt: "2026-03-05T20:20:00Z",
      },
    ]);

    render(<AllPokemonPage pokemonResultsApi={pokemonResultsApi} useSessionHook={createSessionHook()} />);

    await waitFor(() => {
      expect(screen.getByText("Aipom")).toBeTruthy();
      expect(screen.getByText("Zubat")).toBeTruthy();
    });

    const rows = screen.getAllByTestId("scan-row");
    expect(rows[0]).toHaveTextContent("Aipom");

    const sortSelect = screen.getByLabelText("Sort");
    fireEvent.change(sortSelect, { target: { value: "scanDateAsc" } });
    expect(screen.getAllByTestId("scan-row")[0]).toHaveTextContent("Zubat");

    fireEvent.change(sortSelect, { target: { value: "rankAsc" } });
    const rowsByBestRank = screen.getAllByTestId("scan-row");
    expect(rowsByBestRank[0]).toHaveTextContent("Aipom");
  });
});
