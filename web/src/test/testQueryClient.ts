import { QueryClient } from '@tanstack/react-query'

/** Fresh client per call so cache never leaks between tests; no retries keeps failures fast. */
export function createTestQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
}
