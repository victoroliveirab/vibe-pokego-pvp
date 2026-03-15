import { fireEvent, render, screen, within } from "@testing-library/react";
import PokemonResultsPanel from "./PokemonResultsPanel";
import { pokemonResultsPhases } from "./pokemon-results-state";

function createResult(overrides = {}) {
  return {
    id: "result-1",
    speciesName: "Machop",
    cp: 512,
    hp: 64,
    powerUpStardustCost: 2500,
    ivs: {
      attack: 12,
      defense: 15,
      stamina: 13,
    },
    level: {
      estimate: 23.5,
      confidence: 0.72,
      method: "ARC_POSITION",
    },
    source: {
      type: "VIDEO",
      uploadId: "upload-1",
      jobId: "job-1",
      timeRangeMs: {
        start: 12000,
        end: 15500,
      },
      frameTimestampMs: 13200,
    },
    confidence: 0.86,
    maxCpEvaluations: [
      {
        maxCp: 500,
        evaluatedSpeciesId: "machop",
        bestLevel: 10.5,
        bestCp: 500,
        statProduct: 890123.45,
        rank: 9,
        percentage: 89.4,
      },
      {
        maxCp: 1500,
        evaluatedSpeciesId: "machoke",
        bestLevel: 23.5,
        bestCp: 1498,
        statProduct: 1567890.12,
        rank: 143,
        percentage: 93.32,
      },
      {
        maxCp: 2500,
        evaluatedSpeciesId: "machamp",
        bestLevel: 39,
        bestCp: 2499,
        statProduct: 2789012.34,
        rank: 98,
        percentage: 96.11,
      },
    ],
    createdAt: "2026-03-05T20:20:00Z",
    ...overrides,
  };
}

function createProps(overrides = {}) {
  return {
    deleteConfirmation: null,
    deletingResultIds: [],
    error: null,
    lastFetchedAt: "",
    onCancelDeleteResult: vi.fn(),
    onConfirmDeleteResult: vi.fn(),
    onRequestDeleteResult: vi.fn(),
    onRetry: vi.fn(),
    phase: pokemonResultsPhases.IDLE,
    pendingDeleteError: null,
    results: [],
    ...overrides,
  };
}

function createPendingReading(overrides = {}) {
  return {
    id: "reading-1",
    jobId: "job-1",
    uploadId: "upload-1",
    cp: 712,
    hp: 120,
    ivs: {
      attack: 10,
      defense: 11,
      stamina: 12,
    },
    level: {
      estimate: 23.5,
      confidence: 0.72,
      method: "ARC_POSITION",
    },
    source: {
      type: "VIDEO",
      frameTimestampMs: 300,
    },
    confidence: 0.86,
    status: "PENDING_USER_DEDUP",
    createdAt: "2026-03-06T17:00:00Z",
    options: [
      {
        id: "option-1",
        speciesName: "Darumaka",
        matchMode: "exact",
        matchDistance: 0,
        optionRank: 1,
      },
    ],
    ...overrides,
  };
}

describe("pokemon results panel", () => {
  it("renders a loading state while waiting for the first fetch", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.LOADING,
          results: [],
        })}
      />,
    );

    expect(screen.getByText("Loading accepted appraisals...")).toBeTruthy();
  });

  it("renders an explicit empty state when there are no accepted rows", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.SUCCESS,
          results: [],
        })}
      />,
    );

    expect(
      screen.getByText("No accepted appraisals yet. Upload a screenshot or video and wait for processing to finish."),
    ).toBeTruthy();
  });

  it("renders an error state with retry action", () => {
    const onRetry = vi.fn();

    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.ERROR,
          results: [],
          error: {
            code: "INTERNAL_ERROR",
            message: "Server error. Please try again.",
          },
          onRetry,
        })}
      />,
    );

    expect(screen.getByText("Server error. Please try again.")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry results" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("hides IDs by default", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          pendingReadings: [createPendingReading()],
          phase: pokemonResultsPhases.SUCCESS,
          results: [createResult()],
        })}
      />,
    );

    const resultCard = screen.getByRole("heading", { name: "Machop" }).closest("article");
    expect(resultCard).toBeTruthy();

    expect(screen.queryByText("Result ID: result-1")).toBeNull();
    expect(screen.queryByText("Reading reading-1")).toBeNull();
    expect(screen.queryByText("Job job-1 | Upload upload-1")).toBeNull();
    expect(within(resultCard).queryByText("Stardust")).toBeNull();
    expect(within(resultCard).queryByText("Source")).toBeNull();
    expect(within(resultCard).queryByText("Confidence")).toBeNull();
    expect(within(resultCard).queryByText("Created")).toBeNull();
    expect(within(resultCard).getByText("Level")).toBeTruthy();
    expect(within(resultCard).getByText("23.5")).toBeTruthy();
  });

  it("shows IDs when debug mode is enabled", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          isDebugMode: true,
          pendingReadings: [createPendingReading()],
          phase: pokemonResultsPhases.SUCCESS,
          results: [createResult()],
        })}
      />,
    );

    const resultCard = screen.getByRole("heading", { name: "Machop" }).closest("article");
    expect(resultCard).toBeTruthy();

    expect(screen.getByText("Result ID: result-1")).toBeTruthy();
    expect(within(resultCard).getByText("Best fit: machamp @ 2500 CP (96.11%, rank 98)")).toBeTruthy();
    expect(screen.getByText("Reading reading-1")).toBeTruthy();
    expect(screen.getByText("Job job-1 | Upload upload-1")).toBeTruthy();
    expect(within(resultCard).getByText("Stardust")).toBeTruthy();
    expect(within(resultCard).getByText("Source")).toBeTruthy();
    expect(within(resultCard).getByText("Confidence")).toBeTruthy();
    expect(within(resultCard).getByText("Created")).toBeTruthy();
    expect(
      screen.getAllByText("Video | Upload upload-1 | Job job-1 | Time 12000-15500 ms | Frame 13200 ms").length,
    ).toBeGreaterThan(0);
  });

  it("renders tier chips in row headers and null-safe fallbacks", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.SUCCESS,
          lastFetchedAt: "2026-03-05T20:25:00Z",
          results: [
            createResult(),
            createResult({
              id: "result-2",
              speciesName: "Pikachu",
              cp: 410,
              hp: 58,
              powerUpStardustCost: 3000,
              ivs: {
                attack: 10,
                defense: 12,
                stamina: 11,
              },
              level: {
                estimate: null,
                confidence: null,
                method: "UNKNOWN",
              },
              source: {
                type: "IMAGE",
                uploadId: "upload-2",
                jobId: "job-2",
                timeRangeMs: {
                  start: null,
                  end: null,
                },
                frameTimestampMs: null,
              },
              confidence: null,
              maxCpEvaluations: [],
              createdAt: "2026-03-05T20:26:00Z",
            }),
          ],
        })}
      />,
    );

    expect(screen.getAllByLabelText("Best tier for row Machop: S").length).toBeGreaterThan(0);
    expect(screen.getAllByLabelText("Best tier for row Pikachu: N/A").length).toBeGreaterThan(0);

    const machopCard = screen.getByRole("heading", { name: "Machop" }).closest("article");
    const pikachuCard = screen.getByRole("heading", { name: "Pikachu" }).closest("article");
    expect(machopCard).toBeTruthy();
    expect(pikachuCard).toBeTruthy();
    expect(within(machopCard).queryByText("Best fit: machamp @ 2500 CP (96.11%, rank 98)")).toBeNull();
    expect(within(pikachuCard).queryByText("Best fit: N/A")).toBeNull();

    const machopToggle = screen.getAllByRole("button", { name: "Toggle league breakdown row for Machop" })[0];
    const pikachuToggle = screen.getAllByRole("button", { name: "Toggle league breakdown row for Pikachu" })[0];

    expect(machopToggle.disabled).toBe(false);
    expect(pikachuToggle.disabled).toBe(true);
    expect(screen.getByText("Last updated: 2026-03-05T20:25:00Z")).toBeTruthy();
  });

  it("renders a toggled sub-row with Little/Great/Ultra tabs and sorted entries", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.SUCCESS,
          results: [
            createResult({
              speciesName: "Horsea",
              maxCpEvaluations: [
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "kingdra",
                  bestLevel: 21.5,
                  bestCp: 1480,
                  statProduct: 2222222,
                  rank: 246,
                  percentage: 94.02,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "seadra",
                  bestLevel: 27.5,
                  bestCp: 1491,
                  statProduct: 3333333,
                  rank: 105,
                  percentage: 97.46,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "horsea",
                  bestLevel: 50,
                  bestCp: 1038,
                  statProduct: 1111111,
                  rank: 1633,
                  percentage: 60.15,
                },
                {
                  maxCp: 500,
                  evaluatedSpeciesId: "horsea",
                  bestLevel: 10,
                  bestCp: 497,
                  statProduct: 500000,
                  rank: 500,
                  percentage: 70,
                },
                {
                  maxCp: 2500,
                  evaluatedSpeciesId: "kingdra",
                  bestLevel: 44,
                  bestCp: 2498,
                  statProduct: 4000000,
                  rank: 300,
                  percentage: 90,
                },
              ],
            }),
          ],
        })}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Toggle league breakdown row for Horsea" }));

    const region = screen.getByRole("region", { name: "League breakdown row for Horsea" });
    expect(within(region).getByRole("tab", { name: "Little (1)" })).toBeTruthy();
    expect(within(region).getByRole("tab", { name: "Great (3)" })).toBeTruthy();
    expect(within(region).getByRole("tab", { name: "Ultra (1)" })).toBeTruthy();

    fireEvent.click(within(region).getByRole("tab", { name: "Great (3)" }));

    const entries = within(region).getAllByRole("listitem");
    expect(entries[0].textContent).toContain("Seadra");
    expect(entries[1].textContent).toContain("Kingdra");
    expect(entries[2].textContent).toContain("Horsea");

    expect(within(region).getByLabelText("Tier C for Seadra")).toBeTruthy();
    expect(within(region).getByLabelText("Tier D for Kingdra")).toBeTruthy();
    expect(within(region).getByLabelText("Tier F for Horsea")).toBeTruthy();
  });

  it("defaults to the league tab with the best percentage", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.SUCCESS,
          results: [
            createResult({
              speciesName: "Rhyhorn",
              maxCpEvaluations: [
                {
                  maxCp: 500,
                  evaluatedSpeciesId: "littlemon",
                  bestLevel: 10,
                  bestCp: 500,
                  statProduct: 1000,
                  rank: 50,
                  percentage: 95,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "greatmon",
                  bestLevel: 25,
                  bestCp: 1500,
                  statProduct: 2000,
                  rank: 50,
                  percentage: 96,
                },
                {
                  maxCp: 2500,
                  evaluatedSpeciesId: "ultramon",
                  bestLevel: 40,
                  bestCp: 2500,
                  statProduct: 3000,
                  rank: 50,
                  percentage: 97,
                },
              ],
            }),
          ],
        })}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Toggle league breakdown row for Rhyhorn" }));

    const region = screen.getByRole("region", { name: "League breakdown row for Rhyhorn" });
    const ultraTab = within(region).getByRole("tab", { name: "Ultra (1)" });
    expect(ultraTab.getAttribute("aria-pressed")).toBe("true");

    const entries = within(region).getAllByRole("listitem");
    expect(entries).toHaveLength(1);
    expect(entries[0].textContent).toContain("Ultramon");
  });

  it("resets to best tab when closing and reopening a row accordion", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.SUCCESS,
          results: [
            createResult({
              speciesName: "Rhyhorn",
              maxCpEvaluations: [
                {
                  maxCp: 500,
                  evaluatedSpeciesId: "littlemon",
                  bestLevel: 10,
                  bestCp: 500,
                  statProduct: 1000,
                  rank: 50,
                  percentage: 95,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "greatmon",
                  bestLevel: 25,
                  bestCp: 1500,
                  statProduct: 2000,
                  rank: 50,
                  percentage: 96,
                },
                {
                  maxCp: 2500,
                  evaluatedSpeciesId: "ultramon",
                  bestLevel: 40,
                  bestCp: 2500,
                  statProduct: 3000,
                  rank: 50,
                  percentage: 97,
                },
              ],
            }),
          ],
        })}
      />,
    );

    const toggle = screen.getByRole("button", { name: "Toggle league breakdown row for Rhyhorn" });

    fireEvent.click(toggle);
    let region = screen.getByRole("region", { name: "League breakdown row for Rhyhorn" });

    fireEvent.click(within(region).getByRole("tab", { name: "Little (1)" }));
    expect(within(region).getByRole("tab", { name: "Little (1)" }).getAttribute("aria-pressed")).toBe("true");

    fireEvent.click(toggle);
    fireEvent.click(toggle);

    region = screen.getByRole("region", { name: "League breakdown row for Rhyhorn" });
    expect(within(region).getByRole("tab", { name: "Ultra (1)" }).getAttribute("aria-pressed")).toBe("true");
  });

  it("allows multiple expanded rows simultaneously", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.SUCCESS,
          results: [
            createResult({ id: "result-1", speciesName: "Machop" }),
            createResult({ id: "result-2", speciesName: "Dragonite" }),
          ],
        })}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Toggle league breakdown row for Machop" }));
    fireEvent.click(screen.getByRole("button", { name: "Toggle league breakdown row for Dragonite" }));

    expect(screen.getByRole("region", { name: "League breakdown row for Machop" })).toBeTruthy();
    expect(screen.getByRole("region", { name: "League breakdown row for Dragonite" })).toBeTruthy();
  });

  it("maps rank buckets to S/A/B/C/D/F chips", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.SUCCESS,
          results: [
            createResult({
              speciesName: "Bulbasaur",
              maxCpEvaluations: [
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "alpha",
                  bestLevel: 20,
                  bestCp: 1400,
                  statProduct: 1000,
                  rank: 10,
                  percentage: 99,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "bravo",
                  bestLevel: 20,
                  bestCp: 1390,
                  statProduct: 950,
                  rank: 11,
                  percentage: 98,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "charlie",
                  bestLevel: 20,
                  bestCp: 1380,
                  statProduct: 940,
                  rank: 51,
                  percentage: 97,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "delta",
                  bestLevel: 20,
                  bestCp: 1370,
                  statProduct: 930,
                  rank: 101,
                  percentage: 96,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "echo",
                  bestLevel: 20,
                  bestCp: 1360,
                  statProduct: 920,
                  rank: 201,
                  percentage: 95,
                },
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "foxtrot",
                  bestLevel: 20,
                  bestCp: 1350,
                  statProduct: 910,
                  rank: 401,
                  percentage: 94,
                },
              ],
            }),
          ],
        })}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Toggle league breakdown row for Bulbasaur" }));

    const region = screen.getByRole("region", { name: "League breakdown row for Bulbasaur" });

    expect(screen.getAllByLabelText("Best tier for row Bulbasaur: S").length).toBeGreaterThan(0);
    expect(within(region).getByLabelText("Tier S for Alpha")).toBeTruthy();
    expect(within(region).getByLabelText("Tier A for Bravo")).toBeTruthy();
    expect(within(region).getByLabelText("Tier B for Charlie")).toBeTruthy();
    expect(within(region).getByLabelText("Tier C for Delta")).toBeTruthy();
    expect(within(region).getByLabelText("Tier D for Echo")).toBeTruthy();
    expect(within(region).getByLabelText("Tier F for Foxtrot")).toBeTruthy();
  });

  it("renders pending species options and triggers resolve callback", () => {
    const onResolvePendingOption = vi.fn();
    render(
      <PokemonResultsPanel
        {...createProps({
          onResolvePendingOption,
          pendingReadings: [createPendingReading()],
        })}
      />,
    );

    expect(screen.getByText("Pending Species Confirmation")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Darumaka/ }));
    expect(onResolvePendingOption).toHaveBeenCalledWith("reading-1", "option-1");
  });

  it("opens delete confirmation flow for accepted results", () => {
    const onRequestDeleteResult = vi.fn();
    render(
      <PokemonResultsPanel
        {...createProps({
          onRequestDeleteResult,
          phase: pokemonResultsPhases.SUCCESS,
          results: [createResult()],
        })}
      />,
    );

    fireEvent.click(screen.getAllByRole("button", { name: "Delete Machop" })[0]);
    expect(onRequestDeleteResult).toHaveBeenCalledWith(expect.objectContaining({ id: "result-1", speciesName: "Machop" }));
  });

  it("renders delete confirmation modal and wires cancel/confirm actions", () => {
    const onCancelDeleteResult = vi.fn();
    const onConfirmDeleteResult = vi.fn();

    render(
      <PokemonResultsPanel
        {...createProps({
          deleteConfirmation: {
            isOpen: true,
            resultId: "result-1",
            speciesName: "Machop",
          },
          onCancelDeleteResult,
          onConfirmDeleteResult,
          phase: pokemonResultsPhases.SUCCESS,
          results: [createResult()],
        })}
      />,
    );

    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByText("Delete result?")).toBeTruthy();
    expect(screen.getByRole("dialog").textContent).toContain("Delete Machop? This hides the accepted appraisal from future lists.");

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancelDeleteResult).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onConfirmDeleteResult).toHaveBeenCalledTimes(1);
  });

  it("disables delete actions while a result is deleting and shows modal error", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          deleteConfirmation: {
            isOpen: true,
            resultId: "result-1",
            speciesName: "Machop",
          },
          deletingResultIds: ["result-1"],
          pendingDeleteError: {
            code: "INTERNAL_ERROR",
            message: "Delete failed.",
          },
          phase: pokemonResultsPhases.SUCCESS,
          results: [createResult()],
        })}
      />,
    );

    expect(screen.getAllByRole("button", { name: "Delete Machop" })[0].disabled).toBe(true);
    expect(screen.getByRole("button", { name: "Deleting..." }).disabled).toBe(true);
    expect(screen.getByText("Delete failed.")).toBeTruthy();
  });
});
