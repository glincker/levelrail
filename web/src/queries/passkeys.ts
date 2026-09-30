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
import { goAfterLogin, type AuthUser } from './auth'

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

function assertPasskeySupport(): void {
  if (typeof window === 'undefined' || !window.PublicKeyCredential) {
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

// loginWithPasskey drives the public sign-in ceremony: username-first
// (handleBeginPasskeyLogin looks up that account's own credentials),
// not usernameless/discoverable, the same tradeoff the backend's own
// doc comment explains.
export async function loginWithPasskey(username: string): Promise<AuthUser> {
  assertPasskeySupport()
  const beginRes = await fetch('/api/v1/auth/passkey-login/begin', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username }),
  })
  if (!beginRes.ok) {
    await throwPasskeyError(
      beginRes,
      `begin passkey login failed: ${beginRes.status}`,
    )
  }
  const begin = (await beginRes.json()) as PasskeyLoginBeginResponse

  const options = PublicKeyCredential.parseRequestOptionsFromJSON(
    begin.options.publicKey,
  )
  const credential = await navigator.credentials.get({ publicKey: options })
  if (!(credential instanceof PublicKeyCredential)) {
    throw new PasskeyUnsupportedError('The browser did not return a passkey.')
  }

  const finishRes = await fetch(
    `/api/v1/auth/passkey-login/finish?session_id=${encodeURIComponent(begin.session_id)}`,
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
  return (await finishRes.json()) as AuthUser
}

export function useLoginWithPasskey() {
  const navigate = useNavigate()
  const router = useRouter()
  return useMutation<AuthUser, Error, string>({
    mutationFn: loginWithPasskey,
    onSuccess: (user) => {
      setStoredUsername(user.username)
      goAfterLogin(navigate, router)
    },
  })
}
