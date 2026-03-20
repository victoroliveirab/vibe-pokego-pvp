import { buildAllPokemonRow, buildLeagueBreakdown, dedupePokemonResults, filterLeagueEntriesByRelevance, mapMaxCPToLeague } from "./allPokemonUtils";
import { rankToTier } from "../results/pokemonLeagueDisplayUtils";

describe("all pokemon utils", () => {
  it("maps max CP values to league buckets", () => {
    expect(mapMaxCPToLeague(500)).toBe("little");
    expect(mapMaxCPToLeague(1500)).toBe("great");
    expect(mapMaxCPToLeague(2500)).toBe("ultra");
    expect(mapMaxCPToLeague(1000)).toBeNull();
  });

  it("builds and sorts league breakdown entries", () => {
    const byLeague = buildLeagueBreakdown([
      {
        maxCp: 1500,
        evaluatedSpeciesId: "great-b",
        bestLevel: 23,
        bestCp: 1498,
        statProduct: 1200,
        rank: 12,
        percentage: 87.1,
      },
      {
        maxCp: 1500,
        evaluatedSpeciesId: "great-a",
        bestLevel: 23,
        bestCp: 1492,
        statProduct: 1200,
        rank: 8,
        percentage: 92.3,
      },
    ]);

    expect(byLeague.little).toHaveLength(0);
    expect(byLeague.ultra).toHaveLength(0);
    expect(byLeague.great[0].evaluatedSpeciesId).toBe("great-a");
    expect(byLeague.great[1].evaluatedSpeciesId).toBe("great-b");
  });

  it("filters great and ultra entries by the configured best CP thresholds", () => {
    expect(
      filterLeagueEntriesByRelevance(
        [
          { bestCp: 1399, evaluatedSpeciesId: "below-great-threshold" },
          { bestCp: 1400, evaluatedSpeciesId: "at-great-threshold" },
        ],
        "great",
      ).map((entry) => entry.evaluatedSpeciesId),
    ).toEqual(["at-great-threshold"]);

    expect(
      filterLeagueEntriesByRelevance(
        [
          { bestCp: 2249, evaluatedSpeciesId: "below-ultra-threshold" },
          { bestCp: 2250, evaluatedSpeciesId: "at-ultra-threshold" },
        ],
        "ultra",
      ).map((entry) => entry.evaluatedSpeciesId),
    ).toEqual(["at-ultra-threshold"]);

    expect(
      filterLeagueEntriesByRelevance(
        [
          { bestCp: 320, evaluatedSpeciesId: "little-a" },
          { bestCp: 500, evaluatedSpeciesId: "little-b" },
        ],
        "little",
      ).map((entry) => entry.evaluatedSpeciesId),
    ).toEqual(["little-a", "little-b"]);
  });

  it("builds a league row from only threshold-passing entries", () => {
    const row = buildAllPokemonRow(
      {
        id: "result-1",
        speciesName: "Bulbasaur",
        cp: 500,
        hp: 80,
        ivs: { attack: 1, defense: 2, stamina: 3 },
        level: { estimate: 10.5 },
        createdAt: "2026-03-05T10:00:00Z",
        maxCpEvaluations: [
          {
            maxCp: 1500,
            evaluatedSpeciesId: "below-threshold",
            bestLevel: 20,
            bestCp: 1399,
            statProduct: 1200,
            rank: 1,
            percentage: 99.1,
          },
          {
            maxCp: 1500,
            evaluatedSpeciesId: "passing-threshold",
            bestLevel: 22,
            bestCp: 1400,
            statProduct: 1190,
            rank: 8,
            percentage: 95.4,
          },
        ],
      },
      "great",
    );

    expect(row.activeLeagueEntries).toHaveLength(1);
    expect(row.bestActiveEntry.evaluatedSpeciesId).toBe("passing-threshold");
    expect(row.bestTier).toBe("S");
  });

  it("deduplicates by pokemon identity and keeps the latest scan date", () => {
    const resultA = {
      id: "result-a",
      speciesName: "Pikachu",
      cp: 500,
      hp: 80,
      ivs: { attack: 1, defense: 2, stamina: 3 },
      level: { estimate: 10.5 },
      createdAt: "2026-03-05T10:00:00Z",
      maxCpEvaluations: [],
    };
    const resultB = {
      ...resultA,
      id: "result-b",
      createdAt: "2026-03-05T11:00:00Z",
    };

    expect(dedupePokemonResults([resultA, resultB]).map((record) => record.id)).toEqual(["result-b"]);
  });

  it("maps rank tiers consistently for shared tier chips", () => {
    expect(rankToTier(5)).toBe("S");
    expect(rankToTier(35)).toBe("A");
    expect(rankToTier(75)).toBe("B");
    expect(rankToTier(150)).toBe("C");
    expect(rankToTier(350)).toBe("D");
    expect(rankToTier(700)).toBe("F");
  });
});
