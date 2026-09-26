import type { PlatformImportItem } from '../queries/platformImport'

export function isSelectable(item: PlatformImportItem): boolean {
  return (
    (item.kind === 'app' || item.kind === 'database') &&
    (item.status === 'mapped' || item.status === 'needs-attention')
  )
}

export function itemKey(item: PlatformImportItem): string {
  return `${item.kind}:${item.source_id}`
}
