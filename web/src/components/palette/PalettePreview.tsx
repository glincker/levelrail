import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import type { PaletteItem } from '../commandPaletteData'
import { Kbd } from '../commandPaletteEntries'

export const PREVIEW_ID = 'command-palette-preview'
const KEY_ENTER = 'Enter'
const KEY_TAB = 'Tab'

function previewKey(key: string): string {
  return key.replace(/^(recent-|related-|suggested-quick-|suggested-)/, '')
}

interface Props {
  item: PaletteItem | undefined
  onRun: (item: PaletteItem) => void
  onSecondary: (item: PaletteItem) => void
}

// Right-hand card for the highlighted result; the list stays the source of truth for selection.
export function PalettePreview({ item, onRun, onSecondary }: Props) {
  const { t } = useTranslation('palette')
  if (!item) {
    return (
      <aside
        id={PREVIEW_ID}
        aria-label={t('preview.label')}
        className="w-72 shrink-0 border-l border-border p-4 text-xs text-muted-foreground"
      >
        {t('preview.none')}
      </aside>
    )
  }
  const pv = item.preview
  const kind = pv?.kind ?? 'action'
  const description =
    pv?.description ||
    t(`preview.page.${previewKey(item.key)}`, {
      defaultValue: t(`preview.generic.${kind}`),
    })
  const primary = pv?.primaryLabel ?? t('preview.open')
  return (
    <aside
      id={PREVIEW_ID}
      aria-label={t('preview.label')}
      className="flex max-h-80 w-72 shrink-0 flex-col gap-3 overflow-y-auto border-l border-border bg-muted/20 p-4"
    >
      <div className="flex items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-border bg-background text-foreground [&_svg]:size-5">
          {item.icon}
        </span>
        <div className="min-w-0">
          <p className="truncate text-sm font-medium text-foreground">
            {item.label}
          </p>
          <p className="text-xs text-muted-foreground">
            {t(`preview.kind.${kind}`)}
          </p>
        </div>
      </div>
      <p className="text-xs leading-relaxed text-muted-foreground">
        {description}
      </p>
      {pv?.facts && pv.facts.length > 0 ? (
        <ul className="flex flex-wrap gap-1">
          {pv.facts.map((f) => (
            <li
              key={f}
              className="rounded-full border border-border bg-background px-2 py-0.5 text-[11px] text-muted-foreground"
            >
              {f}
            </li>
          ))}
        </ul>
      ) : null}
      {pv?.path ? (
        <p className="truncate font-mono text-[11px] text-muted-foreground">
          {pv.path}
        </p>
      ) : null}
      <div className="mt-auto flex flex-wrap items-center gap-2">
        <Button size="xs" tabIndex={-1} onClick={() => onRun(item)}>
          {primary}
          <Kbd>{KEY_ENTER}</Kbd>
        </Button>
        {item.secondary ? (
          <Button
            size="xs"
            variant="outline"
            tabIndex={-1}
            onClick={() => onSecondary(item)}
          >
            {item.secondaryHint}
            <Kbd>{KEY_TAB}</Kbd>
          </Button>
        ) : null}
      </div>
    </aside>
  )
}
