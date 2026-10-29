import { useTranslation } from 'react-i18next'
import { BellGlassIcon, SirenGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from '@/components/kit'
import {
  useStartFirewallAttack,
  useStopFirewallAttack,
  type FwStatus,
} from '../../queries/firewallEdge'

/** AlertsCard raises the attack-mode suggestion with the fix as a button, or says nothing needs attention. */
export function AlertsCard({ status }: { status: FwStatus }) {
  const { t } = useTranslation('firewall')
  const start = useStartFirewallAttack()
  const stop = useStopFirewallAttack()
  const { attack_mode: attack, attack_suggestion: s } = status

  function fail(e: Error) {
    toast.add({
      title: t('actions.failed'),
      description: e.message,
      type: 'error',
    })
  }

  return (
    <Card className="min-w-0">
      <CardHeader className="border-b">
        <CardTitle>{t('alerts.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        {attack.active && attack.until ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="flex items-center gap-2 text-sm">
              <SirenGlassIcon
                aria-hidden="true"
                className="size-4.5 text-tone-warning"
              />
              {t('alerts.attackOn')} <RelativeTime at={attack.until} />
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={stop.isPending}
              onClick={() => stop.mutate(undefined, { onError: fail })}
            >
              {t('attack.turnOff')}
            </Button>
          </div>
        ) : s.suggested ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="min-w-0 space-y-0.5">
              <p className="flex items-center gap-2 text-sm font-medium">
                <SirenGlassIcon
                  aria-hidden="true"
                  className="size-4.5 text-tone-warning"
                />
                {t('alerts.spikeTitle')}
              </p>
              <p className="text-xs text-muted-foreground">
                {t('alerts.spikeHelp', {
                  rate: s.per_minute,
                  threshold: s.threshold,
                  minutes: s.window_minutes,
                })}
              </p>
            </div>
            <Button
              type="button"
              size="sm"
              disabled={start.isPending}
              onClick={() =>
                start.mutate(
                  { minutes: 0, challenge: false },
                  { onError: fail },
                )
              }
            >
              {t('alerts.turnOn')}
            </Button>
          </div>
        ) : (
          <EmptyState
            compact
            icon={<BellGlassIcon className="size-5.5" />}
            title={t('alerts.noneTitle')}
            description={t('alerts.noneHelp', { threshold: s.threshold })}
          />
        )}
      </CardContent>
    </Card>
  )
}
