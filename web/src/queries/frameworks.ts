import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { BrandIconName } from '../components/BrandIcon'

export interface FrameworkEnvHint {
  key: string
  description: string
  required: boolean
  secret: boolean
  default?: string
}

export interface FrameworkVolumeHint {
  name: string
  path: string
  purpose: string
}

// Matches internal/frameworks.Preset.
export interface FrameworkPreset {
  id: string
  name: string
  runtime: string
  icon: BrandIconName
  build_type: 'railpack' | 'static' | 'dockerfile'
  output_mode: 'server' | 'standalone' | 'static' | 'jar' | 'binary'
  port: number
  health_path: string
  build_command?: string
  start_command?: string
  ready_timeout?: string
  dockerfile_template: boolean
  env_hints: FrameworkEnvHint[]
  volumes: FrameworkVolumeHint[]
  docs_anchor: string
}

export const frameworkKeys = {
  all: ['frameworks'] as const,
}

async function fetchFrameworks(): Promise<FrameworkPreset[]> {
  const res = await fetch('/api/v1/frameworks')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list frameworks failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { frameworks: FrameworkPreset[] }
  return body.frameworks
}

export function frameworksQueryOptions() {
  return queryOptions({
    queryKey: frameworkKeys.all,
    queryFn: fetchFrameworks,
    staleTime: 5 * 60 * 1000,
  })
}

export function useFrameworks() {
  return useQuery(frameworksQueryOptions())
}

export async function fetchFrameworkDockerfile(id: string): Promise<string> {
  const res = await fetch(
    `/api/v1/frameworks/${encodeURIComponent(id)}/dockerfile`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `get dockerfile failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { dockerfile: string }
  return body.dockerfile
}
