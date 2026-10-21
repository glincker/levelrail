// GET /api/v1/docker/health, POST /api/v1/docker/lint and
// GET /api/v1/apps/{name}/image-status (internal/api/docker_integration.go).

import { useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DockerHealthCheck {
  code: string
  status: 'ok' | 'warn' | 'fail' | 'unknown'
  message: string
  fix_action?: 'free_space' | 'recheck'
}

export interface DockerHealth {
  reachable: boolean
  version?: string
  arch?: string
  os?: string
  runtime_kind?: string
  rootless: boolean
  disk_total_bytes: number
  disk_reclaimable_bytes: number
  checked_at: string
  checks: DockerHealthCheck[]
  unresponsive: boolean
  unresponsive_since?: string
}

export interface DockerLintFinding {
  rule: string
  severity: 'warn' | 'info'
  line: number
  message: string
  fixable: boolean
}

export interface DockerLintResult {
  findings: DockerLintFinding[]
  fixed_dockerfile: string
  dockerignore_suggestion?: string
}

export interface ImageStatus {
  image: string
  source: 'registry' | 'built'
  current_digest?: string
  latest_digest?: string
  update_available: boolean
  image_arch?: string
  node_arch?: string
  arch_mismatch: boolean
  message?: string
}

export const dockerIntegrationKeys = {
  health: ['docker', 'health'] as const,
  imageStatus: (app: string) => ['apps', app, 'image-status'] as const,
}

async function getJson<T>(url: string, failure: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${failure}: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function useDockerHealth() {
  return useQuery({
    queryKey: dockerIntegrationKeys.health,
    queryFn: () =>
      getJson<DockerHealth>('/api/v1/docker/health', 'docker health failed'),
  })
}

const DOCKER_DEGRADED_POLL_MS = 15_000

export function useDockerDegradedPoll() {
  return useQuery({
    queryKey: dockerIntegrationKeys.health,
    queryFn: () =>
      getJson<DockerHealth>('/api/v1/docker/health', 'docker health failed'),
    retry: false,
    refetchInterval: DOCKER_DEGRADED_POLL_MS,
    refetchIntervalInBackground: false,
  })
}

export function useImageStatus(appName: string) {
  return useQuery({
    queryKey: dockerIntegrationKeys.imageStatus(appName),
    queryFn: () =>
      getJson<ImageStatus>(
        `/api/v1/apps/${encodeURIComponent(appName)}/image-status`,
        'image status failed',
      ),
    staleTime: 60_000,
  })
}

export function useLintDockerfile() {
  return useMutation({
    mutationFn: async (input: {
      dockerfile: string
      hasDockerignore: boolean
    }): Promise<DockerLintResult> => {
      const res = await fetch('/api/v1/docker/lint', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          dockerfile: input.dockerfile,
          has_dockerignore: input.hasDockerignore,
        }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `lint failed: ${res.status}`),
        )
      }
      return (await res.json()) as DockerLintResult
    },
  })
}
