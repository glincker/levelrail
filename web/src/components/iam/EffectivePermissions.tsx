import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { iamEffectiveQueryOptions } from '../../queries/iamBuilder'
import type { EffectiveAbility } from '../../queries/iamBuilder'
import { RiskBadge } from './RiskBadge'

function AbilityRow({ a }: { a: EffectiveAbility }) {
  const { t } = useTranslation('iam')
  const [open, setOpen] = useState(false)
  const scope =
    a.total === 0
      ? t('effective.noResources')
      : a.all
        ? t('effective.all', { total: a.total })
        : a.allowed === 0
          ? t('effective.none')
          : t('effective.some', { allowed: a.allowed, total: a.total })
  const names = [...a.apps, ...a.databases]
  return (
    <li className="rounded-lg border border-border">
      <button
        type="button"
        aria-expanded={open}
        disabled={names.length === 0}
        onClick={() => setOpen((v) => !v)}
        className="flex w-full flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-left outline-none focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-default"
      >
        <code className="w-32 shrink-0 text-sm text-foreground">
          {a.ability}
        </code>
        <RiskBadge risk={a.risk} />
        <span className="min-w-0 flex-1 text-sm text-muted-foreground">
          {scope}
        </span>
        {a.granted_by_policy > 0 ? (
          <span className="text-xs text-muted-foreground">
            {t('effective.viaPolicy')}: {a.granted_by_policy}
          </span>
        ) : null}
        {a.denied_by_policy > 0 ? (
          <span className="text-xs text-destructive">
            {t('effective.deniedByPolicy')}: {a.denied_by_policy}
          </span>
        ) : null}
        {names.length > 0 ? (
          <CaretDownIcon
            className={cn(
              'size-3.5 text-muted-foreground transition-transform motion-reduce:transition-none',
              open && 'rotate-180',
            )}
            aria-hidden="true"
          />
        ) : null}
      </button>
      {open ? (
        <div className="space-y-1 border-t border-border px-3 py-2 text-xs text-muted-foreground">
          {a.apps.length > 0 ? (
            <p>
              <span className="font-medium text-foreground">
                {t('effective.apps')}:
              </span>{' '}
              {a.apps.join(', ')}
            </p>
          ) : null}
          {a.databases.length > 0 ? (
            <p>
              <span className="font-medium text-foreground">
                {t('effective.databases')}:
              </span>{' '}
              {a.databases.join(', ')}
            </p>
          ) : null}
        </div>
      ) : null}
    </li>
  )
}

/** EffectivePermissions groups what one principal can do by ability, with how much comes from attached policies. */
export function EffectivePermissions({
  principalType,
  principalId,
}: {
  principalType: string
  principalId: string
}) {
  const { t } = useTranslation('iam')
  const q = useQuery(iamEffectiveQueryOptions(principalType, principalId))
  if (q.isPending) return <Skeleton className="h-40 w-full" />
  if (q.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{q.error.message}</AlertDescription>
      </Alert>
    )
  }
  return (
    <section
      aria-label={t('effective.title', { name: q.data.principal.name })}
      className="space-y-2"
    >
      <h3 className="text-sm font-medium text-foreground">
        {t('effective.title', { name: q.data.principal.name })}
      </h3>
      {q.data.restricted ? (
        <p className="text-xs text-muted-foreground">
          {t('effective.restricted')}
        </p>
      ) : null}
      <ul className="space-y-1.5">
        {q.data.abilities.map((a) => (
          <AbilityRow key={a.ability} a={a} />
        ))}
      </ul>
    </section>
  )
}
