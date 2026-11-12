import { useTranslation } from 'react-i18next'
import type { PaletteItem } from '../commandPaletteData'

export function previewKey(key: string): string {
  return key.replace(/^(recent-|related-|suggested-quick-|suggested-)/, '')
}

export function pageText(
  t: (k: string, o: { defaultValue: string }) => string,
  item: PaletteItem,
): string {
  return t(`preview.page.${previewKey(item.key)}`, { defaultValue: '' })
}

/** True when the pane has more to say than the list row already does, so an empty pane can collapse. */
export function usePreviewHasContent(item: PaletteItem | undefined): boolean {
  const { t } = useTranslation('palette')
  if (!item) return false
  const pv = item.preview
  return Boolean(
    pv?.description ||
    pv?.path ||
    (pv?.facts && pv.facts.length > 0) ||
    (pv?.actions && pv.actions.length > 0) ||
    pageText(t, item),
  )
}
