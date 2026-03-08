import { createPokemonResultsApi } from "./pokemon-results-api";

describe("pokemon results api", () => {
  it("fetches and normalizes pokemon results payload", async () => {
    const apiClient = {
      request: vi.fn().mockResolvedValue({
        results: [
          {
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
            createdAt: "2026-03-05T18:00:00Z",
          },
          {
            id: "result-2",
            speciesName: "Pikachu",
            cp: 410,
            hp: 64,
            powerUpStardustCost: 3000,
            ivs: {
              attack: 10,
              defense: 12,
              stamina: 11,
            },
            level: {
              method: "UNKNOWN",
            },
            source: {
              type: "IMAGE",
              uploadId: "upload-2",
              jobId: "job-2",
              timeRangeMs: {},
            },
            createdAt: "2026-03-05T18:00:01Z",
          },
        ],
      }),
    };

    const pokemonResultsApi = createPokemonResultsApi({ apiClient });
    const result = await pokemonResultsApi.getPokemonResults({
      sessionId: "session-1",
    });

    expect(result).toEqual({
      results: [
        {
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
          createdAt: "2026-03-05T18:00:00Z",
        },
        {
          id: "result-2",
          speciesName: "Pikachu",
          cp: 410,
          hp: 64,
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
          createdAt: "2026-03-05T18:00:01Z",
        },
      ],
    });

    expect(apiClient.request).toHaveBeenCalledWith(
      "/pokemon",
      expect.objectContaining({
        method: "GET",
        requiresSession: true,
        sessionId: "session-1",
      }),
    );
  });

  it("throws INVALID_RESPONSE when results array is missing", async () => {
    const pokemonResultsApi = createPokemonResultsApi({
      apiClient: {
        request: vi.fn().mockResolvedValue({ ok: true }),
      },
    });

    await expect(
      pokemonResultsApi.getPokemonResults({
        sessionId: "session-1",
      }),
    ).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
  });

  it("throws INVALID_RESPONSE when record fields have invalid shape", async () => {
    const pokemonResultsApi = createPokemonResultsApi({
      apiClient: {
        request: vi.fn().mockResolvedValue({
          results: [
            {
              id: "result-1",
              speciesName: "Machop",
              cp: "512",
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
              createdAt: "2026-03-05T18:00:00Z",
            },
          ],
        }),
      },
    });

    await expect(
      pokemonResultsApi.getPokemonResults({
        sessionId: "session-1",
      }),
    ).rejects.toMatchObject({
      code: "INVALID_RESPONSE",
    });
  });

  it("fetches and normalizes pending species readings payload", async () => {
    const apiClient = {
      request: vi.fn().mockResolvedValue({
        readings: [
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
      }),
    };

    const pokemonResultsApi = createPokemonResultsApi({ apiClient });
    const payload = await pokemonResultsApi.getPendingSpeciesReadings({
      sessionId: "session-1",
    });

    expect(payload.readings).toHaveLength(1);
    expect(payload.readings[0].id).toBe("reading-1");
    expect(payload.readings[0].options).toHaveLength(1);
    expect(payload.readings[0].options[0].id).toBe("option-1");
    expect(apiClient.request).toHaveBeenCalledWith(
      "/pokemon/pending-species",
      expect.objectContaining({
        method: "GET",
        requiresSession: true,
        sessionId: "session-1",
      }),
    );
  });

  it("resolves one pending reading option", async () => {
    const apiClient = {
      request: vi.fn().mockResolvedValue({
        result: {
          id: "result-1",
          speciesName: "Darumaka",
          cp: 712,
          hp: 120,
          powerUpStardustCost: 0,
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
            uploadId: "upload-1",
            jobId: "job-1",
            timeRangeMs: {
              start: null,
              end: null,
            },
            frameTimestampMs: 300,
          },
          confidence: 0.86,
          createdAt: "2026-03-06T17:01:00Z",
        },
      }),
    };

    const pokemonResultsApi = createPokemonResultsApi({ apiClient });
    const payload = await pokemonResultsApi.resolvePendingSpeciesReading({
      sessionId: "session-1",
      readingId: "reading-1",
      optionId: "option-1",
    });

    expect(payload.result.id).toBe("result-1");
    expect(payload.result.speciesName).toBe("Darumaka");
    expect(apiClient.request).toHaveBeenCalledWith(
      "/pokemon/pending-species/reading-1",
      expect.objectContaining({
        method: "PATCH",
        requiresSession: true,
        sessionId: "session-1",
      }),
    );
  });
});
