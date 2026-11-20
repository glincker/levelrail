export type PageFetcher = (query: { limit: number; offset: number }) => Promise<unknown>;

export interface PaginateOptions {
  pageSize?: number;
  /** Stop after this many items. */
  maxItems?: number;
}

/** Finds the item array in a list response: a bare array or the first array property. */
export function extractItems(page: unknown): unknown[] {
  if (Array.isArray(page)) return page;
  if (page && typeof page === "object") {
    const obj = page as Record<string, unknown>;
    for (const key of ["items", "data", "results"]) {
      if (Array.isArray(obj[key])) return obj[key] as unknown[];
    }
    for (const v of Object.values(obj)) if (Array.isArray(v)) return v;
  }
  return [];
}

/** Walks limit/offset pages until a short page. Honors early exit from `for await`. */
export async function* paginate<T = unknown>(fetchPage: PageFetcher, pageSize: number, opts: PaginateOptions = {}): AsyncGenerator<T> {
  const size = Math.max(1, opts.pageSize ?? pageSize);
  let offset = 0;
  let yielded = 0;
  for (;;) {
    const items = extractItems(await fetchPage({ limit: size, offset }));
    for (const item of items) {
      if (opts.maxItems !== undefined && yielded >= opts.maxItems) return;
      yielded++;
      yield item as T;
    }
    if (items.length < size) return;
    offset += items.length;
  }
}

export async function collect<T>(iter: AsyncIterable<T>): Promise<T[]> {
  const out: T[] = [];
  for await (const v of iter) out.push(v);
  return out;
}
