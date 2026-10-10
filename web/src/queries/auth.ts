// Fetchers and hooks for internal/api/auth.go's three interactive routes:
// POST /api/v1/auth/login, POST /api/v1/auth/register, POST
// /api/v1/auth/logout. No SDK: the theauth SDK assumes a fundamentally
// different backend (OAuth/agent-identity) than Levelrail's actual bcrypt
// + server-side session cookie, so this is a plain fetch, following
// packages/dashboard/src/components/login.tsx's shape.
//
// Both login and register set an httpOnly session_token cookie on
// success and return the same {"username": "..."} body
// (internal/api/auth.go's loginResponse), so they share one response type
// and both hooks do the same thing on success: record the username
// locally (lib/authStore.ts) and navigate to /apps.

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { useCallback } from 'react'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { safeReturnPath } from '../lib/connectionState'
import { setStoredUsername, clearStoredUsername } from '../lib/authStore'

export interface AuthUser {
  username: string
}

// The other shape internal/api/auth.go's loginResponse can take: a
// correct password for an account with TOTP enabled, sent back instead
// of a completed sign-in. LoginForm switches to a second step (entering
// a code) when it sees this instead of AuthUser.
export interface MFARequiredResponse {
  mfa_required: true
  mfa_token: string
}

// A correct password from a browser this account has not trusted while
// another session is live: that session must approve it first.
export interface ApprovalRequiredResponse {
  approval_required: true
  approval_id: string
  approval_expires_at: string
  // The number the approving session must pick out of three.
  approval_match: number
}

export type LoginResult =
  AuthUser | MFARequiredResponse | ApprovalRequiredResponse

export function isMFARequired(
  result: LoginResult,
): result is MFARequiredResponse {
  return 'mfa_required' in result && result.mfa_required
}

export function isApprovalRequired(
  result: LoginResult,
): result is ApprovalRequiredResponse {
  return 'approval_required' in result && result.approval_required
}

// Thrown for a 429 specifically, carrying the seconds-to-wait the
// backend's Retry-After header names (internal/api/auth.go's
// handleLogin, backed by ratelimit.go's real exponential backoff: 3 free
// failures, then doubling from 1s up to a 15-minute cap). LoginForm uses
// this to show "too many attempts, try again in Ns" instead of the
// generic invalid-credentials message, per the task's explicit
// requirement not to collapse rate-limiting into a generic error.
export class RateLimitError extends ApiError {
  readonly retryAfterSeconds: number

  constructor(retryAfterSeconds: number, message: string) {
    super(429, message)
    this.name = 'RateLimitError'
    this.retryAfterSeconds = retryAfterSeconds
  }
}

async function throwAuthError(res: Response, fallback: string): Promise<never> {
  const message = await readErrorMessage(res, fallback)
  if (res.status === 429) {
    const header = res.headers.get('Retry-After')
    const parsed = header ? Number.parseInt(header, 10) : Number.NaN
    throw new RateLimitError(Number.isFinite(parsed) ? parsed : 0, message)
  }
  throw new ApiError(res.status, message)
}

export async function login(
  username: string,
  password: string,
): Promise<LoginResult> {
  const res = await fetch('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) {
    await throwAuthError(res, `login failed: ${res.status}`)
  }
  return (await res.json()) as LoginResult
}

// POST /api/v1/auth/2fa/verify (internal/api/twofactor.go's
// handleVerifyTwoFactor): the second step of login for an account with
// TOTP enabled, exchanging the mfa_token login() returned plus a code
// (or recovery code) for a real session. Same response shape and same
// rate-limit (429) handling as login itself.
export interface VerifyTwoFactorRequest {
  mfaToken: string
  code?: string
  recoveryCode?: string
  // Opt-in only: an unchecked box never trusts the browser.
  rememberDevice?: boolean
}

export async function verifyTwoFactor(
  req: VerifyTwoFactorRequest,
): Promise<AuthUser> {
  const res = await fetch('/api/v1/auth/2fa/verify', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      mfa_token: req.mfaToken,
      code: req.code ?? '',
      recovery_code: req.recoveryCode ?? '',
      remember_device: req.rememberDevice === true,
    }),
  })
  if (!res.ok) {
    await throwAuthError(res, `two-factor verification failed: ${res.status}`)
  }
  return (await res.json()) as AuthUser
}

// First-run counterpart to login: creates the first admin and signs in.
// The server requires the one-time setup token printed by the installer.
// 409 means an admin already exists; RegisterForm offers to switch tabs.
export async function register(
  username: string,
  password: string,
  setupToken: string,
): Promise<AuthUser> {
  const res = await fetch('/api/v1/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password, setup_token: setupToken }),
  })
  if (!res.ok) {
    await throwAuthError(res, `registration failed: ${res.status}`)
  }
  return (await res.json()) as AuthUser
}

// consumeSessionLink calls GET
// /api/v1/auth/session-links/{token}/consume (internal/api/session_links.go's
// handleConsumeSessionLink): a single-use, ~2-minute-TTL token minted by
// POST /api/v1/auth/session-links, exchanged here for a real session the
// same way login() does, no password involved. Same response shape as
// login/register, so LoginScreen reuses setStoredUsername/goAfterLogin
// on success.
export async function consumeSessionLink(token: string): Promise<AuthUser> {
  const res = await fetch(
    `/api/v1/auth/session-links/${encodeURIComponent(token)}/consume`,
  )
  if (!res.ok) {
    await throwAuthError(res, `session link consume failed: ${res.status}`)
  }
  return (await res.json()) as AuthUser
}

// Deliberately ignores the response status: the UI's goal state (no
// local session) is reached either way, whether the cookie was still
// valid (204, internal/api/auth.go's handleLogout) or had already
// expired server-side (401, requireAuth rejecting the logout call
// itself). A network failure still throws, since that's the one case
// where the caller can't assume the server-side session was actually
// revoked, but the UI still clears its own local state regardless (see
// useLogout below).
export async function logout(): Promise<void> {
  await fetch('/api/v1/auth/logout', { method: 'POST' })
}

interface Credentials {
  username: string
  password: string
}

interface RegisterCredentials extends Credentials {
  setupToken: string
}

export interface SetupStatus {
  needs_setup: boolean
}

// GET /api/v1/auth/setup-status: public, true until the first admin exists.
export async function fetchSetupStatus(): Promise<SetupStatus> {
  const res = await fetch('/api/v1/auth/setup-status')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `setup status failed: ${res.status}`),
    )
  }
  return (await res.json()) as SetupStatus
}

export function setupStatusQueryOptions() {
  return queryOptions({
    queryKey: ['auth', 'setup-status'] as const,
    queryFn: fetchSetupStatus,
    staleTime: 30_000,
  })
}

// Both hooks pin TError to ApiError (RateLimitError's base class) rather
// than letting it default to the untyped Error TanStack Query normally
// infers: LoginForm/RegisterForm need to narrow on `instanceof
// RateLimitError` and read `.status`, which only typechecks if the
// mutation's error type says so.
// A completed sign-in stores the username and navigates away, exactly
// as before; an mfa_required result does neither, LoginForm's own
// call-level onSuccess (passed to .mutate) is what switches it to the
// second-step UI, this hook has no navigation to do until that second
// step succeeds too.
// Returns to the path a 401 bounced the user from (?redirect=), else home.
// Exported for queries/passkeys.ts's own useLoginWithPasskey, which
// reaches the same "signed in" end state through a different mutation.
export function goAfterLogin(
  navigate: ReturnType<typeof useNavigate>,
  router: ReturnType<typeof useRouter>,
): void {
  const back = safeReturnPath(
    new URLSearchParams(window.location.search).get('redirect'),
  )
  if (back) {
    void router.navigate({ href: back })
    return
  }
  void navigate({ to: '/' })
}

export function useLogin() {
  const navigate = useNavigate()
  const router = useRouter()
  return useMutation<LoginResult, ApiError, Credentials>({
    mutationFn: ({ username, password }) => login(username, password),
    onSuccess: (result) => {
      if (isMFARequired(result) || isApprovalRequired(result)) {
        return
      }
      setStoredUsername(result.username)
      goAfterLogin(navigate, router)
    },
  })
}

// Records a completed sign-in from any flow (code, approval) and navigates.
export function useFinishSignIn() {
  const navigate = useNavigate()
  const router = useRouter()
  return useCallback(
    (username: string) => {
      setStoredUsername(username)
      goAfterLogin(navigate, router)
    },
    [navigate, router],
  )
}

// useConsumeSessionLink mirrors useLogin's own onSuccess shape exactly
// (record the username, go to wherever ?redirect= or home points), the
// resulting session is a normal login as far as the SPA's own client
// state is concerned.
export function useConsumeSessionLink() {
  const navigate = useNavigate()
  const router = useRouter()
  return useMutation<AuthUser, ApiError, string>({
    mutationFn: consumeSessionLink,
    onSuccess: (user) => {
      setStoredUsername(user.username)
      goAfterLogin(navigate, router)
    },
  })
}

export function useVerifyTwoFactor() {
  const navigate = useNavigate()
  const router = useRouter()
  return useMutation<AuthUser, ApiError, VerifyTwoFactorRequest>({
    mutationFn: verifyTwoFactor,
    onSuccess: (user) => {
      setStoredUsername(user.username)
      goAfterLogin(navigate, router)
    },
  })
}

export function useRegister() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  return useMutation<AuthUser, ApiError, RegisterCredentials>({
    mutationFn: ({ username, password, setupToken }) =>
      register(username, password, setupToken),
    onSuccess: (user) => {
      queryClient.setQueryData(setupStatusQueryOptions().queryKey, {
        needs_setup: false,
      })
      setStoredUsername(user.username)
      void navigate({ to: '/' })
    },
  })
}

export function useLogout() {
  const navigate = useNavigate()
  return useMutation({
    mutationFn: logout,
    // Always clears local state and navigates away, even if the mutation
    // itself throws (a network error talking to /auth/logout should not
    // strand the operator on a page that still thinks it's authenticated
    // when they just asked to sign out).
    onSettled: () => {
      clearStoredUsername()
      void navigate({ to: '/login' })
    },
  })
}
