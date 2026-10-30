// Single source for the settings groups so the nav, the flow strip and the
// group sections cannot drift apart.
export const SETTINGS_GROUP_IDS = [
  'build',
  'run',
  'safety',
  'automation',
] as const

export type SettingsGroupId = (typeof SETTINGS_GROUP_IDS)[number]

export function cardAnchor(card: string): string {
  return `card-${card}`
}

// Keys of deploySettings.cards in common.json; typed so t() stays checked.
export type SettingsCardKey =
  | 'buildHooks'
  | 'buildCache'
  | 'supplyChain'
  | 'secretScan'
  | 'port'
  | 'strategy'
  | 'hooks'
  | 'safety'
  | 'watch'
  | 'autoRollback'
  | 'predeploySnapshot'
  | 'sloBurn'
  | 'canary'
  | 'freeze'
  | 'scheduled'
  | 'cancelSuperseded'
  | 'preview'
  | 'imageAutoUpdate'
  | 'badge'
  | 'githubActions'
  | 'ciDeploy'
