export const SETTINGS_SECTIONS = ['general', 'backups', 'danger'] as const
export type SettingsSection = (typeof SETTINGS_SECTIONS)[number]

// Sections that moved to their own pages; old ?section= links redirect.
export const MOVED_SECTIONS = ['networking', 'credentials'] as const
export type MovedSection = (typeof MOVED_SECTIONS)[number]
