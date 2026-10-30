import { useTranslation } from 'react-i18next'
import { CountdownGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from '@/components/kit'
import {
  useFirewallBans,
  useLiftFirewallBan,
  type FwBan,
} from '../../queries/firewallEdge'
import { formatCount } from './firewallFormat'
import { useFirewallLabels } from './useFirewallLabels'
import { VirtualRows } from './VirtualRows'

const COLS =
  'grid grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_4rem_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-3 px-2 py-2 text-sm'

function BanRow({
  ban,
  onLift,
  lifting,
}: {
  ban: FwBan
  onLift: () => void
  lifting: boolean
}) {
  const { t } = useTranslation('firewall')
  const { reason: reasons } = useFirewallLabels()
  return (
    <div className={COLS}>
      <span className="truncate font-mono text-[13px]" title={ban.ip}>
        {ban.ip}
      </span>
      <span className="truncate text-muted-foreground">
        {reasons[ban.reason] ?? ban.reason}
      </span>
      <span className="text-right tabular-nums">{formatCount(ban.hits)}</span>
      <span className="truncate text-muted-foreground">
        <RelativeTime at={ban.started} />
      </span>
      <span className="truncate text-muted-foreground">
        <RelativeTime at={ban.ends} />
      </span>
      <Button
        type="button"
        variant="outline"
        size="xs"
        disabled={lifting}
        onClick={onLift}
        aria-label={t('bans.liftFor', { ip: ban.ip })}
      >
        {t('bans.lift')}
      </Button>
    </div>
  )
}

/** BansTable lists automatic temporary bans (persistent actions) with start, end and a lift button. */
export function BansTable({
  limit,
  retention,
}: {
  limit?: number
  retention?: string
}) {
  const { t } = useTranslation('firewall')
  const { data, error, isLoading, refetch } = useFirewallBans()
  const lift = useLiftFirewallBan()
  const rows = limit ? (data ?? []).slice(0, limit) : (data ?? [])

  function doLift(ip: string) {
    lift.mutate(ip, {
      onSuccess: () =>
        toast.add({ title: t('bans.lifted', { ip }), type: 'success' }),
      onError: (e) =>
        toast.add({
          title: t('actions.failed'),
          description: e.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card className="min-w-0">
      <CardHeader className="border-b">
        <CardTitle>{t('bans.title')}</CardTitle>
        <CardDescription>{retention ?? t('bans.description')}</CardDescription>
      </CardHeader>
      <CardContent className="px-2">
        {isLoading ? (
          <div aria-hidden="true" className="space-y-2 px-2 py-2">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-5 w-full" />
            ))}
          </div>
        ) : error ? (
          <EmptyState
            compact
            icon={<CountdownGlassIcon className="size-6" />}
            title={t('bans.loadFailed')}
            description={error.message}
            action={
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void refetch()}
              >
                {t('retry')}
              </Button>
            }
          />
        ) : rows.length === 0 ? (
          <EmptyState
            compact
            icon={<CountdownGlassIcon className="size-6" />}
            title={t('bans.emptyTitle')}
            description={t('bans.emptyHelp')}
          />
        ) : (
          <>
            <div
              className={`${COLS} text-xs text-muted-foreground`}
              aria-hidden="true"
            >
              <span>{t('bans.ip')}</span>
              <span>{t('bans.reason')}</span>
              <span className="text-right">{t('bans.requests')}</span>
              <span>{t('bans.started')}</span>
              <span>{t('bans.ends')}</span>
              <span className="w-12" />
            </div>
            <VirtualRows
              items={rows}
              rowHeight={40}
              label={t('bans.title')}
              getKey={(b) => b.ip}
              renderRow={(b) => (
                <BanRow
                  ban={b}
                  lifting={lift.isPending}
                  onLift={() => doLift(b.ip)}
                />
              )}
            />
          </>
        )}
      </CardContent>
    </Card>
  )
}
