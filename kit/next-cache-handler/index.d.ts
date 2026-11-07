export interface CacheHandlerOptions {
  /** Redis client compatible with node-redis v4 or v5. Defaults to a lazily created client from env. */
  client?: RedisLike;
  redisUrl?: string;
  /** Key namespace per app. Env LEVELRAIL_NEXT_CACHE_PREFIX, default "next". */
  keyPrefix?: string;
  /** Build id folded into every key. Env NEXT_BUILD_ID. */
  buildId?: string;
  timeoutMs?: number;
  maxValueBytes?: number;
  lruEntries?: number;
  graceSeconds?: number;
  defaultTtlSeconds?: number;
  retryMs?: number;
  logger?: (message: string) => void;
  now?: () => number;
}

export interface RedisLike {
  get(key: string): Promise<string | null>;
  set(key: string, value: string, options?: { EX?: number }): Promise<unknown>;
  del(key: string): Promise<unknown>;
  hSet(key: string, fields: Record<string, string>): Promise<unknown>;
  hmGet(key: string, fields: string[]): Promise<Array<string | null>>;
  connect?(): Promise<unknown>;
  on?(event: string, listener: (err: Error) => void): unknown;
}

export interface CacheGetContext {
  tags?: string[];
  softTags?: string[];
  kind?: string;
  kindHint?: string;
}

export interface CacheSetContext {
  revalidate?: number | false;
  tags?: string[];
  fetchCache?: boolean;
  fetchUrl?: string;
  fetchIdx?: number;
}

export interface CacheEntry {
  value: unknown;
  lastModified: number;
}

export class CacheHandler {
  constructor(options?: CacheHandlerOptions);
  get(key: string, ctx?: CacheGetContext): Promise<CacheEntry | null>;
  set(key: string, data: unknown, ctx?: CacheSetContext): Promise<void>;
  revalidateTag(tags: string | string[], durations?: unknown): Promise<void>;
  updateTags(tags: string | string[], durations?: unknown): Promise<void>;
  resetRequestCache(): void;
}

export default CacheHandler;
