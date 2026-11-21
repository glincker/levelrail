// UX gate only: the server decides every cloud account action. Writes and
// provider calls are root only today, so that is all the buttons mirror.
export interface CloudAccountActions {
  create: boolean
  verify: boolean
  probe: boolean
  edit: boolean
  remove: boolean
  /** Grants list and edit; a share holder may pass server side even without root. */
  share: boolean
}

export function cloudAccountActions(isRoot: boolean): CloudAccountActions {
  return {
    create: isRoot,
    verify: isRoot,
    probe: isRoot,
    edit: isRoot,
    remove: isRoot,
    share: isRoot,
  }
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
