import { ApiError, readErrorMessage } from './apiError'

export interface ApiRequestOptions {
  method?: string
  body?: unknown
  signal?: AbortSignal
  // Text before ": <status>" in the error shown when the server sends no message.
  fallbackMessage: string
}

// Shared JSON fetch for query modules: JSON headers only when a body is sent,
// ApiError on a non-2xx response, null on 204.
export async function apiRequest<T>(
  path: string,
  { method, body, signal, fallbackMessage }: ApiRequestOptions,
): Promise<T> {
  const hasBody = body !== undefined
  const res = await fetch(path, {
    method,
    headers: hasBody ? { 'Content-Type': 'application/json' } : undefined,
    body: hasBody ? JSON.stringify(body) : undefined,
    signal,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${fallbackMessage}: ${res.status}`),
    )
  }
  if (res.status === 204) return null as T
  return (await res.json()) as T
}
