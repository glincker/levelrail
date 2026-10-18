const memory = new Set<string>()
const PREFIX = 'fixit-auto:'

/** claimAutoApply returns true once per subject per browser session, so a page load never loops a fix. */
export function claimAutoApply(fixId: string, subject: string): boolean {
  const key = `${PREFIX}${fixId}:${subject}`
  if (memory.has(key)) return false
  memory.add(key)
  try {
    if (window.sessionStorage.getItem(key)) return false
    window.sessionStorage.setItem(key, '1')
  } catch {
    return true
  }
  return true
}

/** resetAutoApplyForTests clears the in-memory claims. */
export function resetAutoApplyForTests(): void {
  memory.clear()
}
