// Bulk domain lifecycle. Route lives in internal/api/routes_parked_domains.go.
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import {
  runChunked,
  type BatchItemResult,
  type BatchRequest,
  type BatchResponse,
} from '../lib/domainsBatch'
import { appKeys } from './apps'
import { domainKeys } from './domains'
import { parkedDomainKeys } from './parkedDomains'

export const DOMAINS_BATCH_URL = '/api/v1/domains/batch'

async function sendBatch(req: BatchRequest): Promise<BatchResponse> {
  const res = await fetch(DOMAINS_BATCH_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `batch failed: ${res.status}`),
    )
  }
  return (await res.json()) as BatchResponse
}

export function useDomainsBatch() {
  const qc = useQueryClient()
  return useMutation<BatchItemResult[], Error, BatchRequest>({
    meta: { errorToast: false },
    mutationFn: (req) => runChunked(req, sendBatch),
    onSuccess: (results) => {
      if (!results.some((r) => r.ok === true)) return undefined
      return Promise.all([
        qc.invalidateQueries({ queryKey: parkedDomainKeys.all }),
        qc.invalidateQueries({ queryKey: domainKeys.all }),
        qc.invalidateQueries({ queryKey: appKeys.list() }),
      ])
    },
  })
}
