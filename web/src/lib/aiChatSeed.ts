const TTL_MS = 5 * 60_000

let pending: { message: string; expiresAt: number } | null = null

// In-memory only: a seed is meant to survive exactly one client-side
// route change (e.g. dashboard -> AI assistant), never a page reload.
// The TTL guards against a stale seed firing much later (key configured
// after the click, user returns to the page on their own).
export function setAiChatSeed(message: string): void {
  pending = { message, expiresAt: Date.now() + TTL_MS }
}

export function takeAiChatSeed(): string | null {
  const seed = pending
  pending = null
  if (!seed || seed.expiresAt < Date.now()) return null
  return seed.message
}

export function resetAiChatSeedForTests(): void {
  pending = null
}
