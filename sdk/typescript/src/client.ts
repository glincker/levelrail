import { readDefaults } from "./defaults.ts";
import { LevelrailApiError, LevelrailNetworkError } from "./errors.ts";
import { operations } from "./generated/operations.ts";
import { createResources, type Resources } from "./generated/resources.ts";
import { collect, paginate, type PaginateOptions } from "./pagination.ts";
import { parseSse, type SseEvent } from "./sse.ts";
import type { CallArgs, CallParams, JsonObject, OperationFn, OperationId, Query, RequestOptions } from "./types.ts";

export interface LevelrailOptions {
  /** Control plane origin. Falls back to LEVELRAIL_API_URL. */
  baseUrl?: string;
  /** API token. Falls back to LEVELRAIL_TOKEN. */
  token?: string;
  fetch?: typeof fetch;
  maxRetries?: number;
  retryBaseMs?: number;
  retryMaxMs?: number;
  timeoutMs?: number;
  headers?: Record<string, string>;
}

export interface RawRequest {
  method: string;
  path: string;
  query?: Query | undefined;
  body?: unknown;
}

const SAFE_METHODS = new Set(["GET", "HEAD", "PUT", "DELETE", "OPTIONS"]);

export class Levelrail {
  readonly apps: Resources["apps"];
  readonly deploys: Resources["deploys"];
  readonly staticSites: Resources["staticSites"];
  readonly nodes: Resources["nodes"];
  readonly builders: Resources["builders"];
  readonly environments: Resources["environments"];
  readonly tokens: Resources["tokens"];
  readonly domains: Resources["domains"];

  readonly #baseUrl: string;
  readonly #token: string | undefined;
  readonly #fetch: typeof fetch;
  readonly #o: Required<Pick<LevelrailOptions, "maxRetries" | "retryBaseMs" | "retryMaxMs" | "timeoutMs">> & { headers: Record<string, string> };
  readonly #pageSize: number;

  constructor(options: LevelrailOptions = {}) {
    const d = readDefaults();
    const base = options.baseUrl ?? d.baseUrl;
    if (!base) throw new Error("baseUrl is required (or set LEVELRAIL_API_URL)");
    this.#baseUrl = base.replace(/\/+$/, "");
    this.#token = options.token ?? d.token;
    this.#fetch = options.fetch ?? ((...a) => globalThis.fetch(...a));
    this.#pageSize = d.pageSize;
    this.#o = {
      maxRetries: options.maxRetries ?? d.maxRetries,
      retryBaseMs: options.retryBaseMs ?? d.retryBaseMs,
      retryMaxMs: options.retryMaxMs ?? d.retryMaxMs,
      timeoutMs: options.timeoutMs ?? d.timeoutMs,
      headers: options.headers ?? {},
    };
    const r = createResources(<K extends OperationId>(id: K) => ((...p: CallParams<K>) => this.call(id, ...p)) as OperationFn<K>);
    this.apps = r.apps;
    this.deploys = r.deploys;
    this.staticSites = r.staticSites;
    this.nodes = r.nodes;
    this.builders = r.builders;
    this.environments = r.environments;
    this.tokens = r.tokens;
    this.domains = r.domains;
  }

  /** Calls any operation by its OpenAPI operationId. Stream operations return an async iterator. */
  call<K extends OperationId>(id: K, ...params: CallParams<K>): ReturnType<OperationFn<K>> {
    const [method, template, , sse] = operations[id];
    const args = (params[0] ?? {}) as { path?: Record<string, string>; query?: Query; body?: unknown };
    const path = template.replace(/\{([^}]+)\}/g, (_, k: string) => {
      const v = args.path?.[k];
      if (v === undefined) throw new Error(`missing path parameter "${k}" for ${id}`);
      return encodeURIComponent(v);
    });
    const req: RawRequest = { method, path, query: args.query, body: args.body };
    const opts = params[1] as RequestOptions | undefined;
    return (sse === 1 ? this.stream(req, opts) : this.request(req, opts)) as ReturnType<OperationFn<K>>;
  }

  /** Sends one JSON request and returns the parsed body (undefined for 204). */
  async request<T = unknown>(req: RawRequest, opts: RequestOptions = {}): Promise<T> {
    const res = await this.#send(req, opts, "application/json", true);
    if (res.status === 204) return undefined as T;
    const text = await res.text();
    if (!text) return undefined as T;
    return (res.headers.get("content-type") ?? "").includes("json") ? (JSON.parse(text) as T) : (text as T);
  }

  /** Opens a server-sent event stream. Abort with opts.signal to stop it. */
  async *stream(req: RawRequest, opts: RequestOptions = {}): AsyncGenerator<SseEvent> {
    const res = await this.#send(req, opts, "text/event-stream", false);
    if (!res.body) return;
    yield* parseSse(res.body);
  }

  /** Iterates every item of a limit/offset list operation. */
  paginate<K extends OperationId, T = unknown>(id: K, args: CallArgs<K> | undefined, opts: PaginateOptions & { request?: RequestOptions } = {}): AsyncGenerator<T> {
    const fetchPage = (q: { limit: number; offset: number }) =>
      (this.call as (id: OperationId, a: unknown, o?: RequestOptions) => Promise<unknown>)(id, { ...args, query: { ...args?.query, ...q } }, opts.request);
    return paginate<T>(fetchPage, this.#pageSize, opts);
  }

  /** Collects every page into one array. */
  listAll<K extends OperationId, T = unknown>(id: K, args?: CallArgs<K>, opts?: PaginateOptions & { request?: RequestOptions }): Promise<T[]> {
    return collect(this.paginate<K, T>(id, args, opts));
  }

  async #send(req: RawRequest, opts: RequestOptions, accept: string, withTimeout: boolean): Promise<Response> {
    const url = this.#url(req);
    const headers: Record<string, string> = { accept, ...this.#o.headers, ...opts.headers };
    if (this.#token) headers.authorization = `Bearer ${this.#token}`;
    if (opts.idempotencyKey) headers["idempotency-key"] = opts.idempotencyKey;
    let body: string | undefined;
    if (req.body !== undefined) {
      body = JSON.stringify(req.body);
      headers["content-type"] = "application/json";
    }
    const retryable5xx = SAFE_METHODS.has(req.method) || Boolean(opts.idempotencyKey);
    const maxRetries = opts.maxRetries ?? this.#o.maxRetries;
    const timeoutMs = opts.timeoutMs ?? this.#o.timeoutMs;

    for (let attempt = 0; ; attempt++) {
      opts.signal?.throwIfAborted();
      const signals = [opts.signal, withTimeout && timeoutMs > 0 ? AbortSignal.timeout(timeoutMs) : undefined].filter((s): s is AbortSignal => Boolean(s));
      const signal = signals.length ? AbortSignal.any(signals) : undefined;
      let res: Response;
      try {
        res = await this.#fetch(url, { method: req.method, headers, body, signal });
      } catch (err) {
        if (opts.signal?.aborted || signal?.aborted) throw signal?.reason ?? err;
        if (retryable5xx && attempt < maxRetries) {
          await sleep(this.#backoff(attempt), opts.signal);
          continue;
        }
        throw new LevelrailNetworkError(err instanceof Error ? err.message : "network error", { cause: err });
      }
      if (res.ok) return res;

      const retryAfter = parseRetryAfter(res.headers.get("retry-after"));
      const retry = res.status === 429 || (retryable5xx && [500, 502, 503, 504].includes(res.status));
      const wait = retryAfter ?? this.#backoff(attempt);
      if (retry && attempt < maxRetries && wait <= this.#o.retryMaxMs) {
        await res.body?.cancel().catch(() => undefined);
        await sleep(wait, opts.signal);
        continue;
      }
      throw new LevelrailApiError(res.status, await readErrorBody(res), retryAfter);
    }
  }

  #backoff(attempt: number): number {
    const ceiling = Math.min(this.#o.retryMaxMs, this.#o.retryBaseMs * 2 ** attempt);
    return Math.floor(ceiling / 2 + Math.random() * (ceiling / 2));
  }

  #url(req: RawRequest): string {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(req.query ?? {})) {
      if (v === undefined) continue;
      for (const item of Array.isArray(v) ? v : [v]) qs.append(k, String(item));
    }
    const s = qs.toString();
    return `${this.#baseUrl}${req.path}${s ? `?${s}` : ""}`;
  }
}

async function readErrorBody(res: Response): Promise<unknown> {
  const text = await res.text().catch(() => "");
  try {
    return JSON.parse(text) as JsonObject;
  } catch {
    return text;
  }
}

/** Retry-After is either delta seconds or an HTTP date. */
export function parseRetryAfter(value: string | null): number | undefined {
  if (!value) return undefined;
  const secs = Number(value);
  if (Number.isFinite(secs)) return Math.max(0, secs * 1000);
  const at = Date.parse(value);
  return Number.isNaN(at) ? undefined : Math.max(0, at - Date.now());
}

function sleep(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) return reject(signal.reason);
    const t = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      clearTimeout(t);
      reject(signal?.reason);
    };
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}
