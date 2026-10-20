import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ClockIcon,
  ProhibitIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import { previewChange } from '../../queries/iamBuilder'
import type {
  PreviewRequest,
  PreviewResult,
  PrincipalChange,
  ResourceDelta,
} from '../../queries/iamBuilder'

const PREVIEW_DEBOUNCE_MS = 350

function DeltaRow({
  delta,
  name,
  kind,
}: {
  delta: ResourceDelta
  name: string
  kind: 'gains' | 'loses'
}) {
  const { t } = useTranslation('iam')
  const [open, setOpen] = useState(false)
  const names = [
    ...delta.apps.map((a) => `app:${a}`),
    ...delta.databases.map((d) => `database:${d}`),
  ]
  const gained = kind === 'gains'
  const Icon = gained ? ArrowUpIcon : ArrowDownIcon
  const tone = gained ? TONE.success : TONE.danger
  return (
    <li className="flex flex-col gap-1">
      <div className="flex items-start gap-2 text-sm">
        <span
          className={cn(
            'mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full',
            tone.soft,
            tone.text,
          )}
        >
          <Icon className="size-3" aria-hidden="true" />
        </span>
        <span className="text-foreground">
          {t(`impact.${kind}`, {
            name,
            ability: delta.ability,
            count: delta.count,
          })}
        </span>
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className="ml-auto shrink-0 text-xs text-muted-foreground underline-offset-2 outline-none hover:underline focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          {t('impact.showResources')}
        </button>
      </div>
      {open ? (
        <p className="ml-7 text-xs break-words text-muted-foreground">
          {names.join(', ')}
          {delta.count > names.length ? ` ${t('impact.tooManyHidden')}` : ''}
        </p>
      ) : null}
    </li>
  )
}

function ChangeBlock({ change }: { change: PrincipalChange }) {
  const { t } = useTranslation('iam')
  const name = change.principal.name
  const unchanged = change.gains.length === 0 && change.losses.length === 0
  return (
    <li className="space-y-1.5 rounded-lg border border-border p-3">
      {unchanged ? (
        <p className="text-sm text-muted-foreground">
          {t('impact.noChange', { name })}
        </p>
      ) : (
        <ul className="space-y-1.5">
          {change.gains.map((d) => (
            <DeltaRow
              key={`g-${d.ability}`}
              delta={d}
              name={name}
              kind="gains"
            />
          ))}
          {change.losses.map((d) => (
            <DeltaRow
              key={`l-${d.ability}`}
              delta={d}
              name={name}
              kind="loses"
            />
          ))}
        </ul>
      )}
      {change.recently_used && change.losses.length > 0 ? (
        <p
          className={cn('flex items-center gap-1.5 text-xs', TONE.warning.text)}
        >
          <ClockIcon className="size-3.5" aria-hidden="true" />
          {t('impact.recentlyUsed')}
        </p>
      ) : null}
    </li>
  )
}

/** ImpactPreview shows, before anything is saved, which principals gain or lose which abilities, and whether the no-root-left guard would refuse it. */
export function ImpactPreview({
  request,
  enabled,
  onResult,
}: {
  request: PreviewRequest
  enabled: boolean
  onResult?: (result: PreviewResult | undefined) => void
}) {
  const { t } = useTranslation('iam')
  const key = useDebouncedValue(JSON.stringify(request), PREVIEW_DEBOUNCE_MS)
  const query = useQuery({
    queryKey: ['iam-preview', key],
    queryFn: () => previewChange(JSON.parse(key) as PreviewRequest),
    enabled,
  })
  const data = query.data

  useEffect(() => {
    onResult?.(enabled ? data : undefined)
  }, [data, enabled, onResult])

  if (!enabled) return null
  return (
    <section
      aria-label={t('impact.title')}
      aria-live="polite"
      className="space-y-2"
    >
      <h3 className="text-sm font-medium text-foreground">
        {t('impact.title')}
      </h3>
      {query.isPending ? <Skeleton className="h-16 w-full" /> : null}
      {query.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{query.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {data?.guard.blocked ? (
        <Alert variant="destructive">
          <ProhibitIcon />
          <AlertDescription>{t('impact.guardBlocked')}</AlertDescription>
        </Alert>
      ) : null}
      {data && data.changes.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('impact.none')}</p>
      ) : null}
      {data && data.changes.length > 0 ? (
        <ul className="space-y-2">
          {data.changes.map((c) => (
            <ChangeBlock
              key={`${c.principal.principal_type}:${c.principal.principal_id}`}
              change={c}
            />
          ))}
        </ul>
      ) : null}
      {data ? (
        <p className="text-xs text-muted-foreground">{data.usage_note}</p>
      ) : null}
    </section>
  )
}
