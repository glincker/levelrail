import { ApiError, readErrorMessage } from './apiError'

export interface CascadeDeleteArgs {
  id: string
  cascade: boolean
}

interface CascadeFailure {
  kind: string
  name: string
  error: string
}

interface CascadeProgress {
  failed?: CascadeFailure[]
}

// DELETE with ?cascade=true answers 207 when some members could not be
// removed. The container is kept in that case and repeating the request
// resumes, so the failures are surfaced as an error instead of a success.
export async function deleteWithCascade(
  path: string,
  { id, cascade }: CascadeDeleteArgs,
  failureMessage: string,
): Promise<void> {
  const query = cascade ? '?cascade=true' : ''
  const res = await fetch(`${path}/${encodeURIComponent(id)}${query}`, {
    method: 'DELETE',
  })
  if (res.status === 207) {
    const body = (await res.json()) as CascadeProgress
    const detail = (body.failed ?? [])
      .map((f) => `${f.kind} ${f.name}: ${f.error}`)
      .join('; ')
    throw new ApiError(res.status, detail || failureMessage)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${failureMessage}: ${res.status}`),
    )
  }
}
