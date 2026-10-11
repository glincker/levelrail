// Fetchers and hooks for internal/api/passkeys.go's WebAuthn routes:
// the account-scoped credential management (list/register/delete) and
// the public sign-in ceremony (begin/finish). The wire shape for a
// ceremony's "options" field is exactly what the WebAuthn spec's own
// PublicKeyCredentialCreationOptionsJSON / PublicKeyCredentialRequestOptionsJSON
// types describe (go-webauthn's protocol package produces spec-standard
// JSON), so this file uses those DOM lib types directly rather than
// redeclaring them, and PublicKeyCredential.parseCreationOptionsFromJSON /
// parseRequestOptionsFromJSON / toJSON() to talk to navigator.credentials
// instead of hand-rolled base64url<->ArrayBuffer conversion.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { setStoredUsername } from '../lib/authStore'
import {
  goAfterLogin,
  isApprovalRequired,
  type ApprovalRequiredResponse,
  type AuthUser,
} from './auth'

export interface PasskeyResource {
  id: string
  label: string
  transports?: string[]
  created_at: string
  last_used_at?: string
}

export const passkeyKeys = {
  all: ['passkeys'] as const,
  list: () => [...passkeyKeys.all, 'list'] as const,
}

// Thrown when this browser has no WebAuthn support at all, or
// navigator.credentials returned something that isn't a
// PublicKeyCredential: distinct from ApiError so the UI can tell "your
// browser can't do this" apart from a server-side failure.
export class PasskeyUnsupportedError extends Error {
  constructor(message = 'This browser does not support passkeys.') {
    super(message)
    this.name = 'PasskeyUnsupportedError'
  }
}

export function isPasskeySupported(): boolean {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential
}

function assertPasskeySupport(): void {
  if (!isPasskeySupported()) {
    throw new PasskeyUnsupportedError()
  }
}

async function throwPasskeyError(
  res: Response,
  fallback: string,
): Promise<never> {
  throw new ApiError(res.status, await readErrorMessage(res, fallback))
}

// GET /api/v1/auth/passkeys (handleListPasskeys).
export async function fetchPasskeys(): Promise<PasskeyResource[]> {
  const res = await fetch('/api/v1/auth/passkeys')
  if (!res.ok) {
    await throwPasskeyError(res, `fetch passkeys failed: ${res.status}`)
  }
  return (await res.json()) as PasskeyResource[]
}

export function passkeysQueryOptions() {
  return queryOptions({ queryKey: passkeyKeys.list(), queryFn: fetchPasskeys })
}

export function usePasskeys() {
  return useSuspenseQuery(passkeysQueryOptions())
}

interface PasskeyRegistrationBeginResponse {
  session_id: string
  options: { publicKey: PublicKeyCredentialCreationOptionsJSON }
}

// registerPasskey drives the full registration ceremony: asks the
// server for a challenge (handleBeginPasskeyRegistration), triggers the
// browser's navigator.credentials.create() prompt, and sends the result
// to handleFinishPasskeyRegistration.
export async function registerPasskey(label: string): Promise<PasskeyResource> {
  assertPasskeySupport()
  const beginRes = await fetch('/api/v1/auth/passkeys/register/begin', {
    method: 'POST',
  })
  if (!beginRes.ok) {
    await throwPasskeyError(
      beginRes,
      `begin passkey registration failed: ${beginRes.status}`,
    )
  }
  const begin = (await beginRes.json()) as PasskeyRegistrationBeginResponse

  const options = PublicKeyCredential.parseCreationOptionsFromJSON(
    begin.options.publicKey,
  )
  const credential = await navigator.credentials.create({ publicKey: options })
  if (!(credential instanceof PublicKeyCredential)) {
    throw new PasskeyUnsupportedError('The browser did not return a passkey.')
  }

  const finishURL =
    `/api/v1/auth/passkeys/register/finish?session_id=${encodeURIComponent(begin.session_id)}` +
    `&label=${encodeURIComponent(label)}`
  const finishRes = await fetch(finishURL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(credential.toJSON()),
  })
  if (!finishRes.ok) {
    await throwPasskeyError(
      finishRes,
      `finish passkey registration failed: ${finishRes.status}`,
    )
  }
  return (await finishRes.json()) as PasskeyResource
}

export function useRegisterPasskey() {
  const queryClient = useQueryClient()
  return useMutation<PasskeyResource, Error, string>({
    mutationFn: registerPasskey,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: passkeyKeys.all })
    },
  })
}

// DELETE /api/v1/auth/passkeys/{id} (handleDeletePasskey). 204 on
// success, no body to parse.
export async function deletePasskey(id: string): Promise<void> {
  const res = await fetch(`/api/v1/auth/passkeys/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (res.status === 204) {
    return
  }
  await throwPasskeyError(res, `delete passkey failed: ${res.status}`)
}

export function useDeletePasskey() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deletePasskey,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: passkeyKeys.all })
    },
  })
}

interface PasskeyLoginBeginResponse {
  session_id: string
  options: { publicKey: PublicKeyCredentialRequestOptionsJSON }
}

export interface PasskeyLoginChallenge {
  sessionId: string
  options: PublicKeyCredentialRequestOptionsJSON
}

// Thrown by beginPasskeyLogin for both an unknown username and a known
// one with no passkey, same generic 401 handleBeginPasskeyLogin (Go)
// returns for both, so this stays as enumeration-resistant as the
// backend already is.
export class NoPasskeyForAccountError extends Error {
  constructor() {
    super('No passkey is registered for this account.')
    this.name = 'NoPasskeyForAccountError'
  }
}

// beginPasskeyLogin only fetches the challenge, it never touches
// navigator.credentials: the progressive sign-in flow (LoginScreen)
// calls this first to decide whether to offer a passkey prompt or fall
// back to a password field, without committing to the ceremony yet.
export async function beginPasskeyLogin(
  username: string,
): Promise<PasskeyLoginChallenge> {
  const res = await fetch('/api/v1/auth/passkey-login/begin', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username }),
  })
  if (res.status === 401) {
    throw new NoPasskeyForAccountError()
  }
  if (!res.ok) {
    await throwPasskeyError(res, `begin passkey login failed: ${res.status}`)
  }
  const body = (await res.json()) as PasskeyLoginBeginResponse
  return { sessionId: body.session_id, options: body.options.publicKey }
}

// finishPasskeyLogin prompts the platform authenticator for an
// already-fetched challenge and completes the sign-in.
export async function finishPasskeyLogin(
  challenge: PasskeyLoginChallenge,
): Promise<AuthUser | ApprovalRequiredResponse> {
  assertPasskeySupport()
  const options = PublicKeyCredential.parseRequestOptionsFromJSON(
    challenge.options,
  )
  const credential = await navigator.credentials.get({ publicKey: options })
  if (!(credential instanceof PublicKeyCredential)) {
    throw new PasskeyUnsupportedError('The browser did not return a passkey.')
  }

  const finishRes = await fetch(
    `/api/v1/auth/passkey-login/finish?session_id=${encodeURIComponent(challenge.sessionId)}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(credential.toJSON()),
    },
  )
  if (!finishRes.ok) {
    await throwPasskeyError(
      finishRes,
      `passkey sign-in failed: ${finishRes.status}`,
    )
  }
  return (await finishRes.json()) as AuthUser | ApprovalRequiredResponse
}

export function useBeginPasskeyLogin() {
  return useMutation<PasskeyLoginChallenge, Error, string>({
    mutationFn: beginPasskeyLogin,
  })
}

export function useFinishPasskeyLogin() {
  const navigate = useNavigate()
  const router = useRouter()
  return useMutation<
    AuthUser | ApprovalRequiredResponse,
    Error,
    PasskeyLoginChallenge
  >({
    mutationFn: finishPasskeyLogin,
    // A new browser held for approval stays on the sign-in screen to wait.
    onSuccess: (user) => {
      if (isApprovalRequired(user)) {
        return
      }
      setStoredUsername(user.username)
      goAfterLogin(navigate, router)
    },
  })
}
