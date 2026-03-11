import { fireEvent, render, screen } from "@testing-library/react";
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
    error: null,
    lastFetchedAt: "",
    onRetry: vi.fn(),
    phase: pokemonResultsPhases.IDLE,
    results: [],
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

  it("renders mixed video/image rows with required metrics and null-safe fallbacks", () => {
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

    expect(screen.getAllByText("Machop").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Pikachu").length).toBeGreaterThan(0);
    expect(screen.getAllByText("12/15/13").length).toBeGreaterThan(0);
    expect(screen.getAllByText("10/12/11").length).toBeGreaterThan(0);
    expect(screen.getAllByText("23.5 (ARC_POSITION, 72%)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("N/A (UNKNOWN, N/A)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("86%").length).toBeGreaterThan(0);
    expect(screen.getAllByText("N/A").length).toBeGreaterThan(0);
    expect(screen.getByText("Last updated: 2026-03-05T20:25:00Z")).toBeTruthy();
    expect(
      screen.getAllByText("Video | Upload upload-1 | Job job-1 | Time 12000-15500 ms | Frame 13200 ms").length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText("Image | Upload upload-2 | Job job-2 | Time N/A | Frame N/A").length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText("Best fit: machamp @ 2500 CP (96.11%, rank 98)").length,
    ).toBeGreaterThan(0);
    expect(screen.getAllByText("Best fit: N/A").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Raw Max CP Evaluations (2)").length).toBeGreaterThan(0);
    expect(screen.getAllByText("machoke").length).toBeGreaterThan(0);
    expect(screen.getAllByText("1567890.12").length).toBeGreaterThan(0);
  });

  it("uses tie-breakers for best fit selection: percentage, then lower rank, then higher max cp", () => {
    render(
      <PokemonResultsPanel
        {...createProps({
          phase: pokemonResultsPhases.SUCCESS,
          results: [
            createResult({
              maxCpEvaluations: [
                {
                  maxCp: 1500,
                  evaluatedSpeciesId: "alpha",
                  bestLevel: 20,
                  bestCp: 1499,
                  statProduct: 1000,
                  rank: 42,
                  percentage: 95.55,
                },
                {
                  maxCp: 2500,
                  evaluatedSpeciesId: "beta",
                  bestLevel: 35,
                  bestCp: 2499,
                  statProduct: 1200,
                  rank: 42,
                  percentage: 95.55,
                },
              ],
            }),
          ],
        })}
      />,
    );

    expect(screen.getAllByText("Best fit: beta @ 2500 CP (95.55%, rank 42)").length).toBeGreaterThan(0);
  });

  it("renders pending species options and triggers resolve callback", () => {
    const onResolvePendingOption = vi.fn();
    render(
      <PokemonResultsPanel
        {...createProps({
          onResolvePendingOption,
          pendingReadings: [
            {
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
            },
          ],
        })}
      />,
    );

    expect(screen.getByText("Pending Species Confirmation")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /Darumaka/ }));
    expect(onResolvePendingOption).toHaveBeenCalledWith("reading-1", "option-1");
  });
});
