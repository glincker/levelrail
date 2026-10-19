// Mutations for external inference endpoints (internal/api/models_endpoints.go).

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { modelKeys, requestJson } from './models'
import type { CreateModelResponse, ModelResource } from '../types/models'
import type {
  EndpointApiType,
  EndpointTestResult,
} from '../types/modelEndpoints'

const JSON_HEADERS = { 'Content-Type': 'application/json' }

export interface AddEndpointInput {
  name: string
  baseUrl: string
  apiType: EndpointApiType
  credential: string
}

export function useAddEndpoint() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (v: AddEndpointInput) =>
      requestJson<CreateModelResponse>(
        '/api/v1/models',
        {
          method: 'POST',
          headers: JSON_HEADERS,
          body: JSON.stringify({
            name: v.name,
            external: {
              base_url: v.baseUrl,
              api_type: v.apiType,
              ...(v.credential ? { credential: v.credential } : {}),
            },
          }),
        },
        'register endpoint',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modelKeys.all })
    },
  })
}

export function useProbeEndpoint() {
  return useMutation({
    mutationFn: (v: Omit<AddEndpointInput, 'name'>) =>
      requestJson<EndpointTestResult>(
        '/api/v1/model-endpoints/test',
        {
          method: 'POST',
          headers: JSON_HEADERS,
          body: JSON.stringify({
            base_url: v.baseUrl,
            api_type: v.apiType,
            ...(v.credential ? { credential: v.credential } : {}),
          }),
        },
        'test endpoint',
      ),
  })
}

export function useRecheckEndpoint() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) =>
      requestJson<EndpointTestResult>(
        `/api/v1/models/${encodeURIComponent(name)}/endpoint/test`,
        { method: 'POST' },
        'test endpoint',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modelKeys.all })
    },
  })
}

export function useSetEndpointRouting() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (v: {
      name: string
      fallbackModel: string
      fallbackModelRef: string
      latencyBudgetMs: number
    }) =>
      requestJson<ModelResource>(
        `/api/v1/models/${encodeURIComponent(v.name)}/endpoint/routing`,
        {
          method: 'PUT',
          headers: JSON_HEADERS,
          body: JSON.stringify({
            fallback_model: v.fallbackModel,
            fallback_model_ref: v.fallbackModelRef,
            latency_budget_ms: v.latencyBudgetMs,
          }),
        },
        'set routing',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modelKeys.all })
    },
  })
}

export function useSetEndpointCredential() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (v: { name: string; credential: string }) =>
      requestJson<ModelResource>(
        `/api/v1/models/${encodeURIComponent(v.name)}/endpoint/credential`,
        {
          method: 'PUT',
          headers: JSON_HEADERS,
          body: JSON.stringify({ credential: v.credential }),
        },
        'set credential',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modelKeys.all })
    },
  })
}
