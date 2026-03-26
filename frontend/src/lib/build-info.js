export function parseDeployedAtISO(value, { required = true } = {}) {
  const trimmedValue = String(value || "").trim();

  if (!trimmedValue) {
    if (required) {
      throw new Error("VITE_DEPLOYED_AT_ISO is required");
    }

    return "";
  }

  if (Number.isNaN(Date.parse(trimmedValue))) {
    throw new Error("VITE_DEPLOYED_AT_ISO must be a valid ISO-8601 datetime");
  }

  return trimmedValue;
}

export function getBuildInfo(env = import.meta.env) {
  return {
    deployedAtISO: parseDeployedAtISO(env?.VITE_DEPLOYED_AT_ISO, {
      required: Boolean(env?.PROD),
    }),
  };
}

const buildInfo = getBuildInfo();

export function getDeployedAtISO() {
  return buildInfo.deployedAtISO;
}
