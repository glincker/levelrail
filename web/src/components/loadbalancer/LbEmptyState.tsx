import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon, SplitGlassIcon } from '@thesvg/ui-react'
import { EmptyState } from '@/components/ui/empty-state'
import { Button } from '@/components/ui/button'
import {
  LB_PRESETS,
  presetById,
  RECOMMENDED_PRESET_ID,
  type LbPreset,
} from './presets'

function Diagram({ replicas }: { replicas: number }) {
  const { t } = useTranslation('statePlaceholders')
  const shown = Math.max(replicas, 2)
  return (
    <div
      aria-label={t('lb.empty.diagram', { count: shown })}
      role="img"
      className="flex flex-wrap items-center justify-center gap-2 text-xs"
    >
      <span className="rounded-lg border px-2.5 py-1">
        {t('lb.empty.clients')}
      </span>
      <ArrowRightGlassIcon
        className="size-5 text-muted-foreground"
        aria-hidden
      />
      <span className="rounded-lg border border-tone-accent-border bg-tone-accent-soft px-2.5 py-1">
        {t('lb.empty.proxy')}
      </span>
      <ArrowRightGlassIcon
        className="size-5 text-muted-foreground"
        aria-hidden
      />
      <span className="flex gap-1">
        {Array.from({ length: shown }, (_, i) => (
          <span
            key={i}
            className="rounded-lg border px-2 py-1 text-muted-foreground"
          >
            R{i}
          </span>
        ))}
      </span>
    </div>
  )
}

export function LbEmptyState({
  replicas,
  creating,
  onCreate,
  onPreset,
}: {
  replicas: number
  creating: boolean
  onCreate: (preset: LbPreset) => void
  onPreset: (preset: LbPreset) => void
}) {
  const { t } = useTranslation('statePlaceholders')
  const recommended = presetById(RECOMMENDED_PRESET_ID)
  return (
    <div className="space-y-6">
      <EmptyState
        kind="first-run"
        icon={<SplitGlassIcon size="1.4em" />}
        title={t('lb.empty.title')}
        description={t('lb.empty.description')}
        action={
          recommended ? (
            <Button
              type="button"
              disabled={creating}
              onClick={() => onCreate(recommended)}
            >
              {creating ? t('lb.empty.creating') : t('lb.empty.create')}
            </Button>
          ) : undefined
        }
      />
      <Diagram replicas={replicas} />
      <div>
        <h2 className="mb-2 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {t('lb.empty.presets')}
        </h2>
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {LB_PRESETS.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => onPreset(p)}
                className="min-h-(--glinr-control-h) w-full rounded-xl border p-3 text-left outline-none transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring/60"
              >
                <span className="block text-sm font-medium">{p.name}</span>
                <span className="block text-xs text-muted-foreground">
                  {p.useWhen}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
