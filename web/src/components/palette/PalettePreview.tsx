import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { appListQueryOptions } from '../../queries/apps'
import type { PaletteItem, PreviewAction } from '../commandPaletteData'
import { shortcutLabel } from '@/lib/platform'
import { Kbd } from '../commandPaletteEntries'
import { pageText } from './usePreviewHasContent'

export const PREVIEW_ID = 'command-palette-preview'
const KEY_ENTER = 'Enter'
const KEY_TAB = 'Tab'

interface Props {
  item: PaletteItem | undefined
  onRun: (item: PaletteItem) => void
  onSecondary: (item: PaletteItem) => void
  onAction: (item: PaletteItem, action: PreviewAction) => void
}

function appName(item: PaletteItem): string | null {
  const ref = item.searchRef
  return ref?.startsWith('app:') ? ref.slice(4) : null
}

// Status, URL and image of an app, read from the list the dashboard already cached so the preview costs no request.
function AppFacts({ name }: { name: string }) {
  const { t } = useTranslation('palette')
  const qc = useQueryClient()
  const app = qc
    .getQueryData(appListQueryOptions().queryKey)
    ?.find((a) => a.name === name)
  if (!app) return null
  const url = app.domains?.[0]
  const rows: [string, string][] = [
    [t('preview.status'), app.status.label],
    ...(url ? ([[t('preview.url'), url]] as [string, string][]) : []),
    [t('preview.image'), app.image],
  ]
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs">
      {rows.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-muted-foreground">{k}</dt>
          <dd className="truncate font-mono text-foreground">{v}</dd>
        </div>
      ))}
    </dl>
  )
}

// Right-hand card for the highlighted result; the list stays the source of truth for selection.
export function PalettePreview({ item, onRun, onSecondary, onAction }: Props) {
  const { t } = useTranslation('palette')
  if (!item) return null
  const pv = item.preview
  const kind = pv?.kind ?? 'action'
  const description =
    pv?.description || pageText(t, item) || t(`preview.generic.${kind}`)
  const primary = pv?.primaryLabel ?? t('preview.open')
  const name = appName(item)
  return (
    <aside
      id={PREVIEW_ID}
      aria-label={t('preview.label')}
      className="flex min-h-0 w-80 shrink-0 flex-col border-s border-border bg-muted/20"
    >
      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        <div className="flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-md border border-border bg-background text-foreground [&_svg]:size-6">
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
        {name ? <AppFacts name={name} /> : null}
        {pv?.facts && pv.facts.length > 0 ? (
          <ul className="flex flex-wrap gap-1">
            {pv.facts.map((f) => (
              <li
                key={f}
                className="rounded-full border border-border bg-background px-2 py-0.5 text-xs text-muted-foreground"
              >
                {f}
              </li>
            ))}
          </ul>
        ) : null}
        {pv?.path ? (
          <p className="truncate font-mono text-xs text-muted-foreground">
            {pv.path}
          </p>
        ) : null}
        {pv?.actions && pv.actions.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {pv.actions.map((a) => (
              <Button
                key={a.id}
                size="xs"
                variant="outline"
                tabIndex={-1}
                aria-keyshortcuts={`Alt+${a.shortcut}`}
                onClick={() => onAction(item, a)}
              >
                {a.label}
                <Kbd>{shortcutLabel('alt', a.shortcut)}</Kbd>
              </Button>
            ))}
          </div>
        ) : null}
      </div>
      <div className="flex shrink-0 items-center gap-2 border-t border-border px-3 py-2">
        <Button size="xs" tabIndex={-1} onClick={() => onRun(item)}>
          {primary}
          <Kbd>{KEY_ENTER}</Kbd>
        </Button>
        {item.secondary ? (
          <Button
            size="xs"
            variant="ghost"
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
