// Fetchers for the CPU mode endpoints (internal/api/models_cpu.go).

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  CpuFitReport,
  CpuFitRequest,
  LocalModelCatalog,
  ModelBenchmark,
} from '../types/modelsCpu'
import { modelKeys, requestJson } from './models'

export const modelCpuKeys = {
  catalog: (nodeId: string) => ['models', 'catalog', nodeId] as const,
  fit: (req: CpuFitRequest) => ['models', 'cpu-fit', req] as const,
}

export function useLocalModelCatalog(nodeId = '') {
  return useQuery({
    queryKey: modelCpuKeys.catalog(nodeId),
    queryFn: () =>
      requestJson<LocalModelCatalog>(
        `/api/v1/model-catalog${nodeId ? `?node_id=${encodeURIComponent(nodeId)}` : ''}`,
        undefined,
        'fetch local model catalog',
      ),
    staleTime: 60_000,
  })
}

export function useCpuFit(req: CpuFitRequest, enabled: boolean) {
  return useQuery({
    queryKey: modelCpuKeys.fit(req),
    queryFn: () =>
      requestJson<CpuFitReport>(
        '/api/v1/models/fit',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
        'check cpu model fit',
      ),
    enabled,
    placeholderData: keepPreviousData,
  })
}

export function useBenchmarkModel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) =>
      requestJson<ModelBenchmark>(
        `/api/v1/models/${encodeURIComponent(name)}/benchmark`,
        { method: 'POST' },
        'benchmark model',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modelKeys.all })
    },
  })
}
