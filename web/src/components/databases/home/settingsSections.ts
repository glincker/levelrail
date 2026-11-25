export const SETTINGS_SECTIONS = [
  'general',
  'networking',
  'credentials',
  'backups',
  'danger',
] as const
export type SettingsSection = (typeof SETTINGS_SECTIONS)[number]
