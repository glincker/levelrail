import type { Ability } from '../../types/token'
import type { PageTab } from './pageTabs'

export type SuccessChipId = 'domain' | 'git' | 'password'

export interface SuccessChip {
  id: SuccessChipId
  tab: PageTab
  ability: Ability
}

// Each chip targets an existing Pages tab and needs write, the same gate the server applies there.
const CHIPS: readonly SuccessChip[] = [
  { id: 'domain', tab: 'domains', ability: 'write' },
  { id: 'git', tab: 'deployments', ability: 'write' },
  { id: 'password', tab: 'settings', ability: 'write' },
]

/** Next-step chips the signed-in user may act on; an unknown user sees none. */
export function visibleSuccessChips(
  can: (ability: Ability) => boolean,
): SuccessChip[] {
  return CHIPS.filter((c) => can(c.ability))
}
