import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { PRESETS, type Preset, type PresetKind } from './presets'

const GROUPS: {
  kinds: PresetKind[]
  titleKey:
    | 'deploy.template.groupStarters'
    | 'deploy.template.groupIdeas'
    | 'deploy.template.groupOwn'
}[] = [
  { kinds: ['starter'], titleKey: 'deploy.template.groupStarters' },
  { kinds: ['idea'], titleKey: 'deploy.template.groupIdeas' },
  { kinds: ['repo', 'image'], titleKey: 'deploy.template.groupOwn' },
]

function PresetCard({
  preset,
  selected,
  onSelect,
}: Readonly<{
  preset: Preset
  selected: boolean
  onSelect: (id: string) => void
}>) {
  const { t } = useTranslation('functions')
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={() => {
        onSelect(preset.id)
      }}
      className={cn(
        'flex min-w-0 items-start gap-3 rounded-lg border border-input p-3 text-left transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
        selected && 'border-primary bg-muted',
      )}
    >
      <preset.Icon
        aria-hidden="true"
        className="mt-0.5 size-5 shrink-0 text-muted-foreground"
      />
      <span className="min-w-0 space-y-0.5">
        <span className="block text-sm font-medium">
          {t(`deploy.presets.${preset.id}.title`)}
        </span>
        <span className="block text-xs text-muted-foreground">
          {t(`deploy.presets.${preset.id}.body`)}
        </span>
      </span>
    </button>
  )
}

export function StepTemplate({
  selectedId,
  onSelect,
}: Readonly<{ selectedId: string; onSelect: (id: string) => void }>) {
  const { t } = useTranslation('functions')
  return (
    <div className="min-w-0 space-y-5">
      <h3 className="text-base font-semibold">
        {t('deploy.template.heading')}
      </h3>
      {GROUPS.map((g) => (
        <section key={g.titleKey} className="space-y-2">
          <h4 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            {t(g.titleKey)}
          </h4>
          <div className="grid gap-2 sm:grid-cols-2">
            {PRESETS.filter((p) => g.kinds.includes(p.kind)).map((p) => (
              <PresetCard
                key={p.id}
                preset={p}
                selected={p.id === selectedId}
                onSelect={onSelect}
              />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
