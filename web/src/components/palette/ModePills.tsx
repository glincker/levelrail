import * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  BookOpenGlassIcon,
  BotGlassIcon,
  HashGlassIcon,
  PackageGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { PaletteModeKind } from '@/lib/paletteMode'
import { modesByUse } from '@/lib/paletteModeUse'

const PILLS: Record<
  PaletteModeKind,
  { prefix: string; glyph: string; icon: React.ReactNode }
> = {
  docs: { prefix: '/', glyph: '/', icon: <BookOpenGlassIcon size="1.2em" /> },
  assistant: { prefix: '@', glyph: '@', icon: <BotGlassIcon size="1.2em" /> },
  tags: { prefix: '#', glyph: '#', icon: <HashGlassIcon size="1.2em" /> },
  docker: {
    prefix: 'docker ',
    glyph: 'docker',
    icon: <PackageGlassIcon size="1.2em" />,
  },
}

// The command prefixes as pills: click one to start that mode. The order follows how often each is used.
export function ModePills({
  active,
  onPick,
}: {
  active: PaletteModeKind | null
  onPick: (kind: PaletteModeKind, prefix: string) => void
}) {
  const { t } = useTranslation('palette')
  const order = React.useMemo(() => modesByUse(), [])
  return (
    <div
      role="group"
      aria-label={t('modes.pillsLabel')}
      className="flex flex-wrap items-center gap-2 px-3 pt-3"
    >
      {order.map((kind) => {
        const pill = PILLS[kind]
        const on = kind === active
        return (
          <button
            key={kind}
            type="button"
            tabIndex={-1}
            aria-pressed={on}
            onClick={() => {
              onPick(kind, pill.prefix)
            }}
            className={cn(
              'flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/60',
              on
                ? 'border-transparent bg-foreground text-background'
                : 'border-border bg-foreground/5 text-muted-foreground hover:bg-foreground/10 hover:text-foreground',
            )}
          >
            {pill.icon}
            <span className="font-medium">{t(`modes.${kind}.pill`)}</span>
            <span className="font-mono opacity-60">{pill.glyph}</span>
          </button>
        )
      })}
    </div>
  )
}
