export interface ApiErrorBody {
  code?: string;
  error?: string;
  fix?: unknown;
  request_id?: string;
}

/** Non-2xx response from the control plane, carrying the API error code and message. */
export class LevelrailApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string | undefined;
  readonly fix: unknown;
  readonly retryAfterMs: number | undefined;
  readonly body: unknown;

  constructor(status: number, body: unknown, retryAfterMs?: number) {
    const parsed = (body && typeof body === "object" ? body : {}) as ApiErrorBody;
    const message = parsed.error ?? (typeof body === "string" && body ? body : `request failed with status ${status}`);
    super(message);
    this.name = "LevelrailApiError";
    this.status = status;
    this.code = parsed.code ?? defaultCode(status);
    this.requestId = parsed.request_id;
    this.fix = parsed.fix;
    this.retryAfterMs = retryAfterMs;
    this.body = body;
  }

  get isRateLimited(): boolean {
    return this.status === 429;
  }
}

/** The request never produced an HTTP response (DNS, refused, reset). */
export class LevelrailNetworkError extends Error {
  constructor(message: string, options?: { cause?: unknown }) {
    super(message, options);
    this.name = "LevelrailNetworkError";
  }
}

function defaultCode(status: number): string {
  if (status === 401) return "unauthorized";
  if (status === 403) return "forbidden";
  if (status === 404) return "not_found";
  if (status === 409) return "conflict";
  if (status === 422) return "validation_failed";
  if (status === 429) return "rate_limited";
  return status >= 500 ? "internal_error" : "request_failed";
}
