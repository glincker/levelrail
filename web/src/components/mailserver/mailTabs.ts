export const MAIL_TABS = [
  'overview',
  'mailboxes',
  'domains',
  'backups',
] as const
export type MailTab = (typeof MAIL_TABS)[number]

const STORAGE_KEY = 'levelrail.mail.tab'

export function parseMailTab(v: unknown): MailTab | undefined {
  return MAIL_TABS.find((x) => x === v)
}

// Per-viewer convenience only: storage may be blocked, so every access is guarded.
export function readStoredMailTab(): MailTab | undefined {
  try {
    return parseMailTab(window.localStorage.getItem(STORAGE_KEY))
  } catch {
    return undefined
  }
}

export function storeMailTab(tab: MailTab): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, tab)
  } catch {
    // Storage unavailable: the tab simply is not remembered.
  }
}
