import { buildLeagueBreakdown, dedupePokemonResults, mapMaxCPToLeague } from "./allPokemonUtils";

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
});
