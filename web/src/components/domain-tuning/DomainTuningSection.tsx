import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  useDomainTuning,
  useResetDomainTuning,
  useSetDomainTuning,
  type DomainTuning,
  type TuningPreset,
} from '../../queries/domainTuning'
import { TUNING_PRESETS, isTuningPreset, presetPayload } from './presets'

type Choice = TuningPreset | 'default'

function Picker({
  tuning,
  appName,
  domain,
}: {
  tuning: DomainTuning
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const set = useSetDomainTuning(appName, domain)
  const reset = useResetDomainTuning(appName, domain)
  const saved: Choice | 'custom' = !tuning.configured
    ? 'default'
    : isTuningPreset(tuning.preset)
      ? tuning.preset
      : 'custom'
  const [choice, setChoice] = useState<Choice | 'custom'>(saved)
  const error = set.error ?? reset.error
  const busy = set.isPending || reset.isPending
  const options: readonly Choice[] = ['default', ...TUNING_PRESETS]

  const save = () => {
    const done = () => toast.add({ title: t('tuning.saved'), type: 'success' })
    if (choice === 'default') {
      reset.mutate(undefined, { onSuccess: done })
    } else if (choice !== 'custom') {
      set.mutate(presetPayload(choice, tuning), { onSuccess: done })
    }
  }

  return (
    <form
      className="space-y-2 rounded-md border border-border bg-muted/30 p-3 text-sm"
      onSubmit={(e) => {
        e.preventDefault()
        if (choice !== saved) save()
      }}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{t('tuning.title')}</span>
        <Badge variant={tuning.configured ? 'default' : 'muted'}>
          {saved === 'custom'
            ? t('tuning.custom')
            : t(`tuning.preset.${saved}.label`)}
        </Badge>
      </div>
      <div
        role="radiogroup"
        aria-label={t('tuning.title')}
        className="grid grid-cols-1 gap-2 sm:grid-cols-5"
      >
        {options.map((o) => (
          <button
            key={o}
            type="button"
            role="radio"
            aria-checked={choice === o}
            onClick={() => {
              setChoice(o)
            }}
            className={
              choice === o
                ? 'min-w-0 rounded-md border border-primary bg-primary/10 p-2 text-left'
                : 'min-w-0 rounded-md border border-border bg-background p-2 text-left hover:bg-muted'
            }
          >
            <span className="block text-xs font-medium">
              {t(`tuning.preset.${o}.label`)}
            </span>
            <span className="block text-xs text-muted-foreground">
              {t(`tuning.preset.${o}.hint`)}
            </span>
          </button>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="submit"
          size="sm"
          disabled={choice === saved || choice === 'custom' || busy}
        >
          {t('tuning.save')}
        </Button>
        <span className="text-xs text-muted-foreground">
          {t('tuning.note', {
            max: tuning.max_timeout_seconds,
            stream: tuning.max_stream_seconds,
          })}
        </span>
      </div>
      {tuning.configured && !tuning.effective ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{t('tuning.notApplied')}</AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}
    </form>
  )
}

// Default export so the domain page can load this section lazily.
export default function DomainTuningSection({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { data, isLoading } = useDomainTuning(appName, domain)
  if (isLoading || !data) {
    return isLoading ? (
      <Skeleton className="h-24 w-full" aria-hidden="true" />
    ) : null
  }
  // Keyed on the saved preset so a save or reset re-prefills the choice.
  return (
    <Picker
      key={`${data.configured}-${data.preset}`}
      tuning={data}
      appName={appName}
      domain={domain}
    />
  )
}
