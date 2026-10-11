import { useTranslation } from 'react-i18next'
import { ArrowClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useDnsDelegation } from '../../queries/dns'
import type { DelegationState } from '../../types/dns'

const STATE_VARIANT: Record<
  DelegationState,
  'success' | 'warning' | 'destructive' | 'muted'
> = {
  delegated: 'success',
  partially_delegated: 'warning',
  delegated_elsewhere: 'destructive',
  not_delegated: 'muted',
}

export function DelegationBadge({ state }: { state: DelegationState }) {
  const { t } = useTranslation('dns')
  return (
    <Badge variant={STATE_VARIANT[state]}>
      {t(`delegation.state.${state}`)}
    </Badge>
  )
}

/** DelegationPanel compares what resolvers report for the domain's NS set with the zone's assigned servers. */
export function DelegationPanel({
  zone,
  compact = false,
}: {
  zone: string
  compact?: boolean
}) {
  const { t } = useTranslation('dns')
  const { data, isPending, isFetching, error, refetch } = useDnsDelegation(zone)

  if (isPending) return <Skeleton className="h-24 w-full" />
  if (error) {
    return (
      <p className="text-sm text-destructive" role="alert">
        {error.message}
      </p>
    )
  }
  const d = data.delegation
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <DelegationBadge state={d.state} />
        <span className="text-sm text-muted-foreground">
          {t(`delegation.hint.${d.state}`)}
        </span>
        <Button
          size="sm"
          variant="outline"
          className="ml-auto"
          disabled={isFetching}
          onClick={() => void refetch()}
        >
          <ArrowClockwiseIcon />
          {isFetching ? t('delegation.checking') : t('delegation.recheck')}
        </Button>
      </div>
      {d.elsewhere && d.elsewhere.length > 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('delegation.elsewhere', { servers: d.elsewhere.join(', ') })}
        </p>
      ) : null}
      {compact ? null : (
        <table className="w-full text-sm">
          <thead className="text-left text-xs text-muted-foreground uppercase">
            <tr>
              <th className="py-1 font-medium">
                {t('delegation.col.resolver')}
              </th>
              <th className="py-1 font-medium">
                {t('delegation.col.verdict')}
              </th>
              <th className="py-1 font-medium">{t('delegation.col.answer')}</th>
            </tr>
          </thead>
          <tbody>
            {d.resolvers.map((r) => (
              <tr key={r.server} className="border-t border-border">
                <td className="py-1.5 font-mono">
                  {r.server === 'system' ? t('delegation.system') : r.server}
                </td>
                <td className="py-1.5">
                  {t(`delegation.verdict.${r.verdict}`)}
                </td>
                <td className="py-1.5 font-mono text-xs break-all">
                  {r.error ??
                    (r.name_servers?.join(', ') || t('delegation.noAnswer'))}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
