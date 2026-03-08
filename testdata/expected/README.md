# e2e Expected Fixtures

This directory defines deterministic fixture expectations for smoke/e2e verification.

## Directory Layout

- `testdata/expected/e2e/*.expected.json`: Scenario-level assertions consumed by `scripts/smoke/e2e.sh`.
- `testdata/fixtures/e2e-fixture-manifest.json`: Versioned asset manifest with checksum verification and expected fixture inventory.

## Scenario Fixture Schema

All e2e fixtures contain a `type` field and a stable assertion payload.

### `type: "session_creation"`

```json
{
  "scenarioId": "session_creation",
  "type": "session_creation",
  "apiExpectation": {
    "statusCode": 201,
    "sessionIdFormat": "uuid_v4"
  }
}
```

### `type: "upload_flow"`

```json
{
  "scenarioId": "image_valid_nidoqueen",
  "type": "upload_flow",
  "request": {
    "mediaPath": "worker/testdata/images/valid__species-nidoqueen__cp-1377__hp-132__iv-10-13-15__lvl-200.jpg",
    "contentType": "image/jpeg"
  },
  "jobExpectation": {
    "terminalStatus": "SUCCEEDED",
    "errorCode": null,
    "errorMessageContains": null
  },
  "pokemonExpectation": {
    "minResults": 1,
    "mustContain": [
      {
        "speciesName": "Nidoqueen",
        "cp": 1377,
        "hp": 132,
        "ivs": { "attack": 10, "defense": 13, "stamina": 15 }
      }
    ],
    "mustNotContainSpecies": []
  }
}
```

### `type: "request_error"`

```json
{
  "scenarioId": "error_invalid_session",
  "type": "request_error",
  "request": {
    "method": "GET",
    "path": "/pokemon",
    "useValidSession": false
  },
  "apiExpectation": {
    "statusCode": 401,
    "errorCode": "INVALID_SESSION"
  }
}
```

### `type: "upload_flow"` (pending-user-dedup example)

```json
{
  "scenarioId": "pending_user_dedup_terminal",
  "type": "upload_flow",
  "request": {
    "mediaPath": "worker/testdata/images/valid__species-darumaka__cp-980__hp-124__iv-7-15-15__lvl-250.png",
    "contentType": "image/png"
  },
  "jobExpectation": {
    "terminalStatus": "PENDING_USER_DEDUP",
    "errorCode": null,
    "errorMessageContains": null
  },
  "pokemonExpectation": {
    "minResults": 0,
    "mustContain": [],
    "mustNotContainSpecies": []
  }
}
```

## Notes

- Expected fixture files are intentionally explicit and deterministic.
- `mustContain` entries are matched against `/pokemon.results[]` by exact species/cp/hp/ivs.
- `mustNotContainSpecies` protects against regressions where filtered or pending-only species leak into aggregated results.
