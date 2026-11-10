// Import side of the database data card: GET/POST /api/v1/databases/{name}/import
// (internal/api/database_import.go). Dump size limits stay server side.

import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { databaseKeys } from './databases'

export interface DatabaseImportCheck {
  database: string
  engine: string
  empty: boolean
}

export interface DatabaseImportResult {
  database: string
  engine: string
  bytes: number
  replaced: boolean
}

function importPath(name: string): string {
  return `/api/v1/databases/${encodeURIComponent(name)}/import`
}

export async function fetchImportCheck(
  name: string,
): Promise<DatabaseImportCheck> {
  const res = await fetch(importPath(name))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `import check failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseImportCheck
}

// Enabled only once a file is chosen, so opening the page costs no request.
export function useImportCheck(name: string, enabled: boolean) {
  return useQuery({
    queryKey: [...databaseKeys.detail(name), 'import-check'] as const,
    queryFn: () => fetchImportCheck(name),
    enabled,
    gcTime: 0,
  })
}

// XMLHttpRequest because fetch cannot report upload progress; the File is
// sent as the body, so the browser streams it from disk instead of buffering.
export function uploadDatabaseImport(
  name: string,
  file: File,
  opts: { force: boolean; confirm: string },
  onProgress: (fraction: number) => void,
): { promise: Promise<DatabaseImportResult>; abort: () => void } {
  const xhr = new XMLHttpRequest()
  const qs = opts.force
    ? `?${new URLSearchParams({ force: 'true', confirm: opts.confirm }).toString()}`
    : ''
  const promise = new Promise<DatabaseImportResult>((resolve, reject) => {
    xhr.open('POST', `${importPath(name)}${qs}`)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && e.total > 0) onProgress(e.loaded / e.total)
    }
    xhr.onerror = () => reject(new ApiError(0, 'network error'))
    xhr.onabort = () => reject(new ApiError(0, 'aborted'))
    xhr.onload = async () => {
      const res = new Response(xhr.responseText, { status: xhr.status })
      if (xhr.status < 200 || xhr.status >= 300) {
        reject(
          new ApiError(
            xhr.status,
            await readErrorMessage(res, `import failed: ${xhr.status}`),
          ),
        )
        return
      }
      resolve((await res.json()) as DatabaseImportResult)
    }
    xhr.send(file)
  })
  return { promise, abort: () => xhr.abort() }
}
