function env(name: string): string | undefined {
  const g = globalThis as { process?: { env?: Record<string, string | undefined> } };
  return g.process?.env?.[name];
}

function num(name: string, fallback: number): number {
  const v = Number(env(name));
  return env(name) !== undefined && Number.isFinite(v) && v >= 0 ? v : fallback;
}

/** Tunables read from the environment so callers can change them without code edits. */
export function readDefaults() {
  return {
    baseUrl: env("LEVELRAIL_API_URL"),
    token: env("LEVELRAIL_TOKEN"),
    maxRetries: num("LEVELRAIL_SDK_MAX_RETRIES", 2),
    retryBaseMs: num("LEVELRAIL_SDK_RETRY_BASE_MS", 400),
    retryMaxMs: num("LEVELRAIL_SDK_RETRY_MAX_MS", 10_000),
    timeoutMs: num("LEVELRAIL_SDK_TIMEOUT_MS", 0),
    pageSize: num("LEVELRAIL_SDK_PAGE_SIZE", 100),
  };
}
