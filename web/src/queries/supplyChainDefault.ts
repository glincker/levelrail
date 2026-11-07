import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const SCAN_GATES = [
  'off',
  'warn',
  'block_on_critical',
  'block_on_high',
] as const
export type ScanGate = (typeof SCAN_GATES)[number]

export interface SupplyChainDefault {
  scan_enabled: boolean
  scan_gate: string
  server_enabled: boolean
}

const URL = '/api/v1/settings/supply-chain'
const KEY = ['settings', 'supply-chain'] as const

export function supplyChainDefaultQueryOptions() {
  return queryOptions({
    queryKey: KEY,
    queryFn: async (): Promise<SupplyChainDefault> => {
      const res = await fetch(URL)
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch scan policy failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as SupplyChainDefault
    },
  })
}

export function useSaveSupplyChainDefault() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (body: {
      scan_enabled: boolean
      scan_gate: ScanGate
    }) => {
      const res = await fetch(URL, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `save scan policy failed: ${res.status}`),
        )
      }
      return (await res.json()) as SupplyChainDefault
    },
    onSuccess: (data) => qc.setQueryData(KEY, data),
  })
}
