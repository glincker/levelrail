import { useMutation } from '@tanstack/react-query'
import { EnvApiError } from './envProtection'

export interface ProjectExportArgs {
  project: string
  includeSecrets: boolean
}

export interface ProjectExportResult {
  blob: Blob
  filename: string
}

export const projectExportKeys = {
  download: (project: string) =>
    ['projectExport', 'download', project] as const,
}

// The server answers with a tar; the filename comes from Content-Disposition
// so it matches what the CLI writes.
function filenameFrom(res: Response, project: string): string {
  const header = res.headers.get('Content-Disposition') ?? ''
  const match = /filename="?([^";]+)"?/.exec(header)
  return match?.[1] ?? `project-${project}-compose.tar`
}

export async function fetchProjectExport({
  project,
  includeSecrets,
}: ProjectExportArgs): Promise<ProjectExportResult> {
  const query = includeSecrets ? '?include_secrets=true' : ''
  const res = await fetch(
    `/api/v1/projects/${encodeURIComponent(project)}/export${query}`,
    { cache: 'no-store' },
  )
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
    } | null
    throw new EnvApiError(
      res.status,
      body?.error ?? `export failed: ${String(res.status)}`,
      body?.code ?? '',
    )
  }
  return { blob: await res.blob(), filename: filenameFrom(res, project) }
}

export function useProjectExport() {
  return useMutation({
    mutationKey: ['projectExport', 'download'],
    mutationFn: fetchProjectExport,
  })
}
