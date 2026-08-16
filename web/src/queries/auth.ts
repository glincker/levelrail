// Fetchers and hooks for internal/api/auth.go's three interactive routes:
// POST /api/v1/auth/login, POST /api/v1/auth/register, POST
// /api/v1/auth/logout. No SDK: docs-local/research/frontend-component-
// reuse.md section 2 is explicit that the theauth SDK assumes a
// fundamentally different backend (OAuth/agent-identity) than Levelrail's
// actual bcrypt + server-side session cookie, so this is a plain fetch,
// following packages/dashboard/src/components/login.tsx's shape from that
// same research doc.
//
// Both login and register set an httpOnly session_token cookie on
// success and return the same {"username": "..."} body
// (internal/api/auth.go's loginResponse), so they share one response type
// and both hooks do the same thing on success: record the username
// locally (lib/authStore.ts) and navigate to /apps.

import { useMutation } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { setStoredUsername, clearStoredUsername } from '../lib/authStore'

export interface AuthUser {
  username: string
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
): Promise<AuthUser> {
  const res = await fetch('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) {
    await throwAuthError(res, `login failed: ${res.status}`)
  }
  return (await res.json()) as AuthUser
}

// First-run counterpart to login: same request/response shape, but also
// creates the single admin row (internal/api/auth.go's handleRegister)
// and auto-logs-in on success, same as login. 409 means an admin already
// exists; RegisterForm surfaces that distinctly and offers to switch to
// the sign-in tab, per the task's explicit UX carve-out for that one
// status code (401 stays deliberately generic, 409 does not).
export async function register(
  username: string,
  password: string,
): Promise<AuthUser> {
  const res = await fetch('/api/v1/auth/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  })
  if (!res.ok) {
    await throwAuthError(res, `registration failed: ${res.status}`)
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

// Both hooks pin TError to ApiError (RateLimitError's base class) rather
// than letting it default to the untyped Error TanStack Query normally
// infers: LoginForm/RegisterForm need to narrow on `instanceof
// RateLimitError` and read `.status`, which only typechecks if the
// mutation's error type says so.
export function useLogin() {
  const navigate = useNavigate()
  return useMutation<AuthUser, ApiError, Credentials>({
    mutationFn: ({ username, password }) => login(username, password),
    onSuccess: (user) => {
      setStoredUsername(user.username)
      void navigate({ to: '/' })
    },
  })
}

export function useRegister() {
  const navigate = useNavigate()
  return useMutation<AuthUser, ApiError, Credentials>({
    mutationFn: ({ username, password }) => register(username, password),
    onSuccess: (user) => {
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
