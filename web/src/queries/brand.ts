// Query-key factory and fetcher for GET /api/v1/brand
// (internal/api/brand.go), the public endpoint the login screen (and now
// every other screen, via BrandProvider) reads the product name from
// instead of a hardcoded string, per the rebrandability rule: no product
// name string may be hardcoded in the frontend.

import { queryOptions } from '@tanstack/react-query'
import type { Brand } from '../types/brand'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const brandKeys = {
  all: ['brand'] as const,
}

export async function fetchBrand(): Promise<Brand> {
  const res = await fetch('/api/v1/brand')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch brand failed: ${res.status}`),
    )
  }
  return (await res.json()) as Brand
}

// staleTime: Infinity because brand.yaml (plus its APP_BRAND_* env
// overrides) is fixed for the lifetime of a running control plane
// process; there is no route in this app that changes it, so refetching
// on every window focus/remount would just be wasted requests. A full
// page reload still gets a fresh value, since the QueryClient itself is
// recreated then.
export function brandQueryOptions() {
  return queryOptions({
    queryKey: brandKeys.all,
    queryFn: fetchBrand,
    staleTime: Infinity,
  })
}
