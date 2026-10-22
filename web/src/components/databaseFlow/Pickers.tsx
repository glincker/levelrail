import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { BrandTile } from '../BrandLogoBadge'
import type { DatabaseEngineInfo } from '../../queries/databaseEngines'
import { PRESETS, type PresetId } from './logic'

const CARD =
  'rounded-lg border border-input text-left transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none'

export function EngineCards({
  engines,
  value,
  onChange,
}: Readonly<{
  engines: readonly DatabaseEngineInfo[]
  value: string
  onChange: (id: string) => void
}>) {
  const { t } = useTranslation('databases')
  return (
    <div
      role="radiogroup"
      aria-label={t('flow.engine')}
      className="grid min-w-0 grid-cols-1 gap-2 min-[480px]:grid-cols-2"
    >
      {engines.map((e) => {
        const selected = e.id === value
        return (
          <button
            key={e.id}
            type="button"
            role="radio"
            aria-checked={selected}
            onClick={() => {
              onChange(e.id)
            }}
            className={cn(
              CARD,
              'flex min-w-0 items-center gap-3 p-2.5',
              selected && 'border-primary bg-muted',
            )}
          >
            <BrandTile name={e.id} label={e.label} size="sm" />
            <span className="min-w-0">
              <span className="block truncate text-sm font-medium">
                {e.label}
              </span>
              <span className="block truncate text-xs text-muted-foreground">
                {t(`flow.engines.${e.id}`, { defaultValue: '' })}
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}

export function PresetCards({
  value,
  onChange,
}: Readonly<{ value: PresetId; onChange: (id: PresetId) => void }>) {
  const { t } = useTranslation('databases')
  return (
    <div
      role="radiogroup"
      aria-label={t('flow.size')}
      className="grid min-w-0 grid-cols-1 gap-2 min-[480px]:grid-cols-3"
    >
      {PRESETS.map((p) => (
        <button
          key={p.id}
          type="button"
          role="radio"
          aria-checked={value === p.id}
          onClick={() => {
            onChange(p.id)
          }}
          className={cn(
            CARD,
            'min-w-0 space-y-0.5 p-3',
            value === p.id && 'border-primary bg-muted',
          )}
        >
          <span className="block text-sm font-medium">
            {t(`flow.presets.${p.id}`)}
          </span>
          <span className="block font-mono text-xs text-muted-foreground">
            {t('flow.presetSpec', { memory: p.memoryMiB, cpus: p.cpus })}
          </span>
        </button>
      ))}
    </div>
  )
}
