// Same-key queries mounted by several components in one page load would each
// refetch at staleTime 0; this window collapses them to a single request.
export const DEDUPE_STALE_MS = 10_000
