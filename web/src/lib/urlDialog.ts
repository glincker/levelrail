// Pure search-param logic behind kit/UrlDialog, kept free of the router
// so the open/close rules are unit-testable on their own.

export type SearchRecord = Record<string, unknown>

export const DEFAULT_DIALOG_PARAM = 'connect'

export function isUrlDialogOpen(
  search: SearchRecord,
  param: string,
  value: string,
): boolean {
  return search[param] === value
}

// Returns the search to navigate to when the dialog opens: the trigger
// param plus any extra params, leaving unrelated params untouched.
export function openUrlDialogSearch(
  search: SearchRecord,
  param: string,
  value: string,
  extra: Record<string, string> = {},
): SearchRecord {
  return { ...search, ...extra, [param]: value }
}

// Closing removes the trigger param and every dialog-owned extra param,
// so a reload or a shared link does not reopen or half-open it.
export function closeUrlDialogSearch(
  search: SearchRecord,
  param: string,
  ownedParams: readonly string[] = [],
): SearchRecord {
  const next: SearchRecord = { ...search }
  delete next[param]
  for (const key of ownedParams) delete next[key]
  return next
}

// validateSearch helper: keeps only non-empty string values for keys.
export function pickStringSearch(
  search: SearchRecord,
  keys: readonly string[],
): Record<string, string> {
  const out: Record<string, string> = {}
  for (const key of keys) {
    const v = search[key]
    if (typeof v === 'string' && v !== '') out[key] = v
  }
  return out
}
