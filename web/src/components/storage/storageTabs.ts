export const STORAGE_TABS = ['images', 'cache'] as const
export type StorageTab = (typeof STORAGE_TABS)[number]

export function parseStorageTab(v: unknown): StorageTab {
  return STORAGE_TABS.find((x) => x === v) ?? 'images'
}
