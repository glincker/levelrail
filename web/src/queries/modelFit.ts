// Queries for POST /api/v1/models/fit and GET /api/v1/models/{name}/fit
// (internal/api/models_fit.go).

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import type { FitReport, FitRequest } from '../types/modelFit'
import { modelKeys, requestJson } from './models'

const FIT_STALE_MS = 15_000

export const modelFitKeys = {
  check: (req: FitRequest) => [...modelKeys.all, 'fit', req] as const,
  model: (name: string) => [...modelKeys.all, 'fit', 'model', name] as const,
}

export function useFitCheck(req: FitRequest | null) {
  return useQuery({
    queryKey: modelFitKeys.check(req ?? { engine: '', model: '' }),
    queryFn: () =>
      requestJson<FitReport>(
        '/api/v1/models/fit',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
        'check model fit',
      ),
    enabled: req !== null,
    staleTime: FIT_STALE_MS,
    placeholderData: keepPreviousData,
  })
}

export function useModelFit(name: string) {
  return useQuery({
    queryKey: modelFitKeys.model(name),
    queryFn: () =>
      requestJson<FitReport>(
        `/api/v1/models/${encodeURIComponent(name)}/fit`,
        undefined,
        'check model fit',
      ),
    staleTime: FIT_STALE_MS,
  })
}
