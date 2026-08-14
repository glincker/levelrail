// Mutation for the secrets sub-resource of an app: PUT
// /api/v1/apps/{name}/secrets/{key} (internal/api/secrets.go's
// handleSetSecret). Deliberately no query-key factory and no GET
// fetcher here, unlike apps.ts/deploys.ts: the backend has no
// corresponding GET by design (a decrypted secret value must never
// round-trip into a browser, per that handler's own doc comment), so
// there is nothing to cache, prime in a loader, or invalidate. This
// module's only job is the write.

import { useMutation } from '@tanstack/react-query'

// Thrown when the control plane responds 501, meaning it was started
// without APP_MASTER_KEY set: handleSetSecret checks rt.secrets == nil
// before it even loads the app or looks at the request body. Distinct
// from every other failure this mutation can produce because it is a
// server configuration gap, not a bad key/value or a missing app, so
// callers should render it as an explanation rather than a form error.
export class SecretsNotConfiguredError extends Error {
  constructor() {
    super(
      'secrets are not configured on this control plane (no master key set)',
    )
    this.name = 'SecretsNotConfiguredError'
  }
}

async function readErrorMessage(
  res: Response,
  fallback: string,
): Promise<string> {
  const body = (await res.json().catch(() => null)) as {
    error?: string
  } | null
  return body?.error ?? fallback
}

export interface SetSecretInput {
  key: string
  value: string
}

// handleSetSecret returns 204 with no body on success (the value just
// submitted is already known to the caller, echoing it back would be an
// unnecessary leak surface), 501 if no master key is configured, 404 if
// the app does not exist, and 400 for an empty value or malformed body.
export async function setSecret(
  appName: string,
  input: SetSecretInput,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/secrets/${encodeURIComponent(input.key)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: input.value }),
    },
  )
  if (res.status === 204) {
    return
  }
  if (res.status === 501) {
    throw new SecretsNotConfiguredError()
  }
  throw new Error(
    await readErrorMessage(res, `set secret failed: ${res.status}`),
  )
}

// No cache write on success (see module doc above: there is nothing to
// cache). Callers own their own form reset and success messaging, same
// shape as useTriggerDeploy's fire-and-forget framing in deploys.ts.
export function useSetSecret(appName: string) {
  return useMutation({
    mutationFn: (input: SetSecretInput) => setSecret(appName, input),
  })
}
