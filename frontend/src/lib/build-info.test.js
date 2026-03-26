import { getBuildInfo, parseDeployedAtISO } from "./build-info";

describe("build info", () => {
  it("returns the configured deployment timestamp", () => {
    expect(parseDeployedAtISO("2026-03-25T14:03:11Z")).toBe("2026-03-25T14:03:11Z");
  });

  it("throws when the deployment timestamp is missing and required", () => {
    expect(() => getBuildInfo({ PROD: true, VITE_DEPLOYED_AT_ISO: "" })).toThrow(
      "VITE_DEPLOYED_AT_ISO is required",
    );
  });

  it("throws when the deployment timestamp is invalid", () => {
    expect(() => parseDeployedAtISO("not-a-date")).toThrow(
      "VITE_DEPLOYED_AT_ISO must be a valid ISO-8601 datetime",
    );
  });

  it("allows the deployment timestamp to be omitted in local dev mode", () => {
    expect(getBuildInfo({ PROD: false, VITE_DEPLOYED_AT_ISO: "" })).toEqual({
      deployedAtISO: "",
    });
  });
});
