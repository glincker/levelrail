// POST /api/v1/apps/{name}/exec (internal/api/exec.go's handleExecApp):
// a non-interactive, run-and-return-output exec into an app's currently
// running container. Deliberately not a query (nothing here is cached
// or invalidation-worthy the way appKeys/deployKeys are): every call is
// a fresh one-off command, so this is a bare useMutation with no query
// key at all, the same "no cache, no key" shape queries/builds.ts's
// useTriggerBuild would have if it didn't also need to update the app
// detail cache afterward. A nonzero exit_code in a 200 response is a
// normal outcome (the command ran and failed on its own terms), not a
// mutation error: only a non-2xx HTTP status (app not found, no running
// container, timeout, exec not configured) throws here, matching
// readErrorMessage's own contract for every other fetcher in this
// directory.

import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface ExecAppInput {
  command: string
  args?: string[]
  timeoutSeconds?: number
}

export interface ExecAppResult {
  stdout: string
  stderr: string
  exit_code: number
  truncated?: boolean
}

export async function execInApp(
  name: string,
  input: ExecAppInput,
): Promise<ExecAppResult> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/exec`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      command: input.command,
      ...(input.args && input.args.length > 0 ? { args: input.args } : {}),
      ...(input.timeoutSeconds
        ? { timeout_seconds: input.timeoutSeconds }
        : {}),
    }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `exec failed: ${res.status}`),
    )
  }
  return (await res.json()) as ExecAppResult
}

export function useExecApp(name: string) {
  return useMutation({
    mutationFn: (input: ExecAppInput) => execInApp(name, input),
  })
}
