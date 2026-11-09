import { ApiError, readErrorMessage } from '../lib/apiError'

export interface BundleService {
  key: string
  name: string
  image: string
}

export interface BundleImportResult {
  app: string
  dry_run: boolean
  sha256: string
  services: BundleService[]
  secrets_to_set?: string[]
  warnings?: string[]
  deployed: boolean
}

export async function importBundle(
  file: Blob,
  name: string,
  dryRun: boolean,
): Promise<BundleImportResult> {
  const q = new URLSearchParams()
  if (name) q.set('name', name)
  if (dryRun) q.set('dry_run', 'true')
  const path = `/api/v1/apps/import-bundle?${q.toString()}`
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/x-tar' },
    body: file,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `POST ${path} failed: ${res.status}`),
    )
  }
  return (await res.json()) as BundleImportResult
}
