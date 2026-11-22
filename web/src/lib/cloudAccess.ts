import type { CloudAccessData } from '../types/cloudAccounts'

// UX gate only: the server decides every cloud account action. Build, delete
// and the account writes stay root only; the rest follow the actions the API
// says the caller holds on the account.
export interface CloudAccountActions {
  create: boolean
  verify: boolean
  probe: boolean
  edit: boolean
  remove: boolean
  build: boolean
  delete: boolean
  view: boolean
  operate: boolean
  manageBudget: boolean
  viewSpend: boolean
  /** Grants list and edit. */
  share: boolean
}

export function cloudAccountActions(
  isRoot: boolean,
  held: readonly string[] = [],
): CloudAccountActions {
  const has = (a: string) => isRoot || held.includes(a)
  return {
    create: isRoot,
    verify: isRoot,
    probe: isRoot,
    edit: isRoot,
    remove: isRoot,
    build: isRoot,
    delete: isRoot,
    view: has('view'),
    operate: has('operate'),
    manageBudget: has('manage_budget'),
    viewSpend: has('view_spend'),
    share: has('share'),
  }
}

// Root comes from the user list or the access payload, whichever settles first.
export function actionsFromAccess(
  isRoot: boolean,
  access: CloudAccessData | undefined,
  accountId: string | undefined,
): CloudAccountActions {
  const root = isRoot || access?.root === true
  const held = accountId ? (access?.accounts[accountId] ?? []) : []
  return cloudAccountActions(root, held)
}

// The delete dialog asks for the account label, falling back to the id.
export function deleteConfirmName(account: {
  id: string
  label: string
}): string {
  return account.label.trim() !== '' ? account.label : account.id
}

export function canConfirmDelete(typed: string, expected: string): boolean {
  return typed.trim() === expected
}
