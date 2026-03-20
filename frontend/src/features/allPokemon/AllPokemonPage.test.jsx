import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import AllPokemonPage from "./AllPokemonPage";

function createSessionHook() {
  return () => ({
    sessionId: "session-1",
    isLoading: false,
    error: null,
  });
}

function createPokemonResultsApi(payloads = [[]]) {
  const responsePayloads = Array.isArray(payloads[0]) ? payloads : [payloads];

  return {
    getPokemonResults: vi.fn().mockImplementation(async () => ({
      results: responsePayloads.length > 1 ? responsePayloads.shift() : responsePayloads[0],
    })),
    deletePokemonResult: vi.fn().mockResolvedValue(null),
  };
}

function createResult({
  confidence = 0.88,
  cp = 300,
  createdAt = "2026-03-05T20:00:00Z",
  hp = 90,
  id,
  ivs = { attack: 12, defense: 12, stamina: 12 },
  level = { estimate: 10, confidence: 0.7, method: "ARC_POSITION" },
  maxCpEvaluations = [],
  powerUpStardustCost = 2500,
  source = {
    type: "IMAGE",
    uploadId: "upload-1",
    jobId: "job-1",
    timeRangeMs: { start: 0, end: 1000 },
    frameTimestampMs: 200,
  },
  speciesName,
} = {}) {
  return {
    id,
    speciesName,
    cp,
    hp,
    ivs,
    level,
    powerUpStardustCost,
    source,
    confidence,
    maxCpEvaluations,
    createdAt,
  };
}

describe("all pokemon page", () => {
  it("filters the active league with strict thresholds and only expands active-league details", async () => {
    const pokemonResultsApi = createPokemonResultsApi([
      createResult({
        id: "result-1",
        speciesName: "Bulbasaur",
        maxCpEvaluations: [
          {
            maxCp: 1500,
            evaluatedSpeciesId: "bulba-great",
            bestLevel: 20,
            bestCp: 1490,
            statProduct: 2,
            rank: 25,
            percentage: 83.0,
          },
          {
            maxCp: 500,
            evaluatedSpeciesId: "bulba-little",
            bestLevel: 12,
            bestCp: 410,
            statProduct: 1.2,
            rank: 50,
            percentage: 92.1,
          },
        ],
        createdAt: "2026-03-05T20:00:00Z",
      }),
      createResult({
        id: "result-2",
        speciesName: "Azurill",
        cp: 220,
        hp: 70,
        maxCpEvaluations: [
          {
            maxCp: 1500,
            evaluatedSpeciesId: "azurill-great",
            bestLevel: 20,
            bestCp: 1399,
            statProduct: 2,
            rank: 3,
            percentage: 97.0,
          },
        ],
        createdAt: "2026-03-05T20:05:00Z",
      }),
      createResult({
        id: "result-3",
        speciesName: "Charmander",
        cp: 350,
        hp: 70,
        ivs: { attack: 10, defense: 11, stamina: 10 },
        level: { estimate: 11, confidence: 0.6, method: "ARC_POSITION" },
        source: {
          type: "VIDEO",
          uploadId: "upload-2",
          jobId: "job-2",
          timeRangeMs: { start: 100, end: 900 },
          frameTimestampMs: 400,
        },
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
        createdAt: "2026-03-05T20:00:00Z",
      }),
      createResult({
        id: "result-4",
        speciesName: "Weedle",
        cp: 100,
        hp: 55,
        maxCpEvaluations: [
          {
            maxCp: 500,
            evaluatedSpeciesId: "weedle-little",
            bestLevel: 18,
            bestCp: 320,
            statProduct: 1.5,
            rank: 80,
            percentage: 88.2,
          },
        ],
        createdAt: "2026-03-05T20:30:00Z",
      }),
    ]);

    render(<AllPokemonPage pokemonResultsApi={pokemonResultsApi} useSessionHook={createSessionHook()} />);

    await waitFor(() => {
      expect(screen.getByText("Bulbasaur")).toBeTruthy();
    });

    expect(screen.queryByText("Charmander")).toBeNull();
    expect(screen.queryByText("Azurill")).toBeNull();
    expect(screen.queryByText("Weedle")).toBeNull();

    const row = screen.getByText("Bulbasaur").closest("[data-testid='scan-row']");
    expect(row).toBeTruthy();
    expect(within(row).getByLabelText("Best Great tier for Bulbasaur: A")).toBeTruthy();
    expect(within(row).getByText("IV 12-12-12 / LVL 10.0 / Little 92.1%")).toBeTruthy();

    fireEvent.click(within(row).getByRole("button", { name: "Show Great details" }));
    expect(within(row).getByText(/Bulba Great/i)).toBeTruthy();
    expect(screen.queryByText(/Bulba Little/i)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Ultra" }));
    await waitFor(() => {
      expect(screen.getByText("Charmander")).toBeTruthy();
    });
    expect(screen.queryByText("Bulbasaur")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Little" }));
    await waitFor(() => {
      expect(screen.getByText("Weedle")).toBeTruthy();
    });
  });

  it("sorts rows with a single combined sort control", async () => {
    const pokemonResultsApi = createPokemonResultsApi([
      createResult({
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
      }),
      createResult({
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
      }),
    ]);

    render(<AllPokemonPage pokemonResultsApi={pokemonResultsApi} useSessionHook={createSessionHook()} />);

    await waitFor(() => {
      const rows = screen.getAllByTestId("scan-row");
      expect(rows).toHaveLength(2);
      expect(rows[0].textContent).toContain("Aipom");
      expect(rows[1].textContent).toContain("Zubat");
    });

    const rows = screen.getAllByTestId("scan-row");
    expect(rows[0].textContent).toContain("Aipom");

    const sortSelect = screen.getByLabelText("Sort");
    fireEvent.change(sortSelect, { target: { value: "scanDateAsc" } });
    expect(screen.getAllByTestId("scan-row")[0].textContent).toContain("Zubat");

    fireEvent.change(sortSelect, { target: { value: "rankDesc" } });
    expect(screen.getAllByTestId("scan-row")[0].textContent).toContain("Zubat");
  });

  it("shows the best overall league summary even when viewing another active league", async () => {
    const pokemonResultsApi = createPokemonResultsApi([
      createResult({
        id: "result-1",
        speciesName: "Horsea",
        ivs: { attack: 0, defense: 15, stamina: 8 },
        level: { estimate: 13, confidence: 0.7, method: "ARC_POSITION" },
        maxCpEvaluations: [
          {
            maxCp: 1500,
            evaluatedSpeciesId: "kingdra-great",
            bestLevel: 22,
            bestCp: 1497,
            statProduct: 2.4,
            rank: 12,
            percentage: 98.5,
          },
          {
            maxCp: 2500,
            evaluatedSpeciesId: "kingdra-ultra",
            bestLevel: 39,
            bestCp: 2300,
            statProduct: 3.4,
            rank: 70,
            percentage: 92.2,
          },
        ],
      }),
    ]);

    render(<AllPokemonPage pokemonResultsApi={pokemonResultsApi} useSessionHook={createSessionHook()} />);

    fireEvent.click(screen.getByRole("button", { name: "Ultra" }));

    await waitFor(() => {
      expect(screen.getByText("Horsea")).toBeTruthy();
    });

    const row = screen.getByText("Horsea").closest("[data-testid='scan-row']");
    expect(row).toBeTruthy();
    expect(within(row).getByText("IV 0-15-8 / LVL 13.0 / Great 98.5%")).toBeTruthy();
  });

  it("keeps debug details hidden by default and reveals them when debug mode is enabled", async () => {
    const pokemonResultsApi = createPokemonResultsApi([
      createResult({
        id: "result-1",
        speciesName: "Bulbasaur",
        maxCpEvaluations: [
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
      }),
    ]);

    render(<AllPokemonPage pokemonResultsApi={pokemonResultsApi} useSessionHook={createSessionHook()} />);

    await waitFor(() => {
      expect(screen.getByText("Bulbasaur")).toBeTruthy();
    });

    expect(screen.queryByText("Result ID")).toBeNull();
    expect(screen.queryByText(/Upload upload-1/i)).toBeNull();

    fireEvent.click(screen.getByLabelText("Debug mode"));

    expect(screen.getByText("Result ID")).toBeTruthy();
    expect(screen.getByText(/Upload upload-1/i)).toBeTruthy();
  });

  it("deletes a result and refreshes the list", async () => {
    const bulbasaur = createResult({
      id: "result-1",
      speciesName: "Bulbasaur",
      maxCpEvaluations: [
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
    });
    const aipom = createResult({
      id: "result-2",
      speciesName: "Aipom",
      createdAt: "2026-03-05T20:20:00Z",
      maxCpEvaluations: [
        {
          maxCp: 1500,
          evaluatedSpeciesId: "aipom-great",
          bestLevel: 20,
          bestCp: 1490,
          statProduct: 2,
          rank: 18,
          percentage: 84.0,
        },
      ],
    });
    const pokemonResultsApi = createPokemonResultsApi([[bulbasaur, aipom], [aipom]]);

    render(<AllPokemonPage pokemonResultsApi={pokemonResultsApi} useSessionHook={createSessionHook()} />);

    await waitFor(() => {
      expect(screen.getAllByTestId("scan-row")).toHaveLength(2);
    });

    fireEvent.click(screen.getByRole("button", { name: "Delete Bulbasaur" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(pokemonResultsApi.deletePokemonResult).toHaveBeenCalledWith({
        sessionId: "session-1",
        resultId: "result-1",
      });
    });

    await waitFor(() => {
      expect(screen.queryByText("Bulbasaur")).toBeNull();
    });
    expect(screen.getByText("Aipom")).toBeTruthy();
    expect(pokemonResultsApi.getPokemonResults).toHaveBeenCalledTimes(2);
  });

  it("shows delete errors and keeps the confirmation dialog open", async () => {
    const pokemonResultsApi = createPokemonResultsApi([
      createResult({
        id: "result-1",
        speciesName: "Bulbasaur",
        maxCpEvaluations: [
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
      }),
    ]);
    pokemonResultsApi.deletePokemonResult = vi.fn().mockRejectedValue({
      message: "Delete failed.",
    });

    render(<AllPokemonPage pokemonResultsApi={pokemonResultsApi} useSessionHook={createSessionHook()} />);

    await waitFor(() => {
      expect(screen.getByText("Bulbasaur")).toBeTruthy();
    });

    fireEvent.click(screen.getByRole("button", { name: "Delete Bulbasaur" }));
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeTruthy();
      expect(screen.getByText("Delete failed.")).toBeTruthy();
    });
  });
});
