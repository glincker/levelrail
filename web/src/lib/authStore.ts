// Client-side mirror of "is there a logged-in session," framework-agnostic
// so it can be read from route beforeLoad hooks (see routes/__root.tsx and
// routes/login.tsx) and from main.tsx's imperative navigation, not just
// from React components (see hooks/useAuthUsername.ts for the React-facing
// wrapper).
//
// The real session lives in an httpOnly cookie the frontend can never
// read (internal/api/auth.go's session_token), so this is only ever a
// heuristic: "did the last login/register call succeed, and has nothing
// cleared that since." It answers docs-local/research/dashboard-gap-audit-
// and-devmode.md gap #4's "no prior successful login recorded" case
// (routes/__root.tsx's beforeLoad guard) without a network round trip on
// every navigation. The actual enforcement is still server-side: any
// route that gets a real 401 back (session expired, server restarted and
// wiped its in-memory session store) clears this and redirects too, via
// the QueryCache/MutationCache handler in main.tsx.
//
// Backed by localStorage (not a plain module variable) so the flag
// survives a full page reload, which a plain in-memory value would not:
// without this, hitting refresh on an authenticated page would flash the
// login screen before any request even had a chance to prove the session
// cookie is still good.

const STORAGE_KEY = 'app-auth-username'

type Listener = () => void

const listeners = new Set<Listener>()

function readFromStorage(): string | null {
  try {
    return window.localStorage.getItem(STORAGE_KEY)
  } catch {
    // localStorage can throw in private-browsing/storage-restricted
    // contexts. Falling back to "no stored session" just means the
    // beforeLoad guard sends the user to /login, which is always a safe
    // default, never a broken one.
    return null
  }
}

let cachedUsername: string | null = readFromStorage()

function writeToStorage(username: string | null): void {
  try {
    if (username === null) {
      window.localStorage.removeItem(STORAGE_KEY)
    } else {
      window.localStorage.setItem(STORAGE_KEY, username)
    }
  } catch {
    // See readFromStorage: cachedUsername still updates in memory below,
    // so auth state stays correct for the rest of this tab's lifetime,
    // it just won't survive a reload.
  }
}

function emit(): void {
  for (const listener of listeners) {
    listener()
  }
}

export function getStoredUsername(): string | null {
  return cachedUsername
}

export function setStoredUsername(username: string): void {
  cachedUsername = username
  writeToStorage(username)
  emit()
}

export function clearStoredUsername(): void {
  cachedUsername = null
  writeToStorage(null)
  emit()
}

// For useSyncExternalStore (hooks/useAuthUsername.ts): notifies listeners
// whenever setStoredUsername/clearStoredUsername runs, so any component
// reading the username (the app shell's nav) re-renders on login/logout
// without needing a shared React context provider for something this
// small.
export function subscribeStoredUsername(listener: Listener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
