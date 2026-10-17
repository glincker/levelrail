import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { SettingRow } from '@/components/ui/setting-row'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { RelativeTime, StatusPill } from '@/components/kit'
import { RuleSelect } from '../rules/RuleSelect'
import {
  useFirewallStatus,
  useStartFirewallAttack,
  useStopFirewallAttack,
} from '../../queries/firewallEdge'
import { DangerZoneCard } from './DangerZoneCard'

const DURATIONS = [15, 60, 240, 720, 1440]

/** AttackModeCard is the Danger Zone switch: tighten every limit instance wide for a bounded time, optionally challenging every visitor. */
export function AttackModeCard() {
  const { t } = useTranslation('firewall')
  const { data } = useFirewallStatus('', '1h')
  const start = useStartFirewallAttack()
  const stop = useStopFirewallAttack()
  const [minutes, setMinutes] = useState(60)
  const [challenge, setChallenge] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const attack = data?.attack_mode

  function turnOn() {
    setError(null)
    start.mutate(
      { minutes, challenge },
      {
        onSuccess: () => {
          setConfirm(false)
          toast.add({ title: t('attack.onToast'), type: 'success' })
        },
        onError: (e) => setError(e.message),
      },
    )
  }

  function turnOff() {
    stop.mutate(undefined, {
      onSuccess: () =>
        toast.add({ title: t('attack.offToast'), type: 'success' }),
      onError: (e) =>
        toast.add({
          title: t('actions.failed'),
          description: e.message,
          type: 'error',
        }),
    })
  }

  return (
    <DangerZoneCard
      title={t('attack.title')}
      description={t('attack.description')}
    >
      <SettingRow
        title={t('attack.switchTitle')}
        description={t('attack.switchHelp')}
        badge={
          attack?.active ? (
            <StatusPill tone="warning" label={t('status.on')} size="sm" />
          ) : undefined
        }
      >
        {attack?.active && attack.until ? (
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-sm text-muted-foreground">
              {t('attack.endsIn')} <RelativeTime at={attack.until} live />
            </span>
            <Button
              type="button"
              variant="outline"
              disabled={stop.isPending}
              onClick={turnOff}
            >
              {t('attack.turnOff')}
            </Button>
          </div>
        ) : (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <RuleSelect
              aria-label={t('attack.duration')}
              value={String(minutes)}
              options={DURATIONS.map((m) => ({
                value: String(m),
                label: t('attack.minutes', { count: m }),
              }))}
              onChange={(e) => setMinutes(Number(e.target.value))}
            />
            <label className="flex items-center gap-2 text-sm">
              <Switch
                size="sm"
                checked={challenge}
                onCheckedChange={setChallenge}
                aria-label={t('attack.challenge')}
              />
              {t('attack.challenge')}
            </label>
            <Button
              type="button"
              variant="destructive"
              onClick={() => setConfirm(true)}
            >
              {t('attack.turnOn')}
            </Button>
          </div>
        )}
      </SettingRow>
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t('attack.confirmTitle')}
        description={t('attack.confirmHelp', { minutes })}
        consequences={[
          t('attack.consequenceLimits'),
          challenge
            ? t('attack.consequenceChallenge')
            : t('attack.consequenceBots'),
        ]}
        confirmLabel={t('attack.turnOn')}
        tone="destructive"
        pending={start.isPending}
        error={error}
        onConfirm={turnOn}
      />
    </DangerZoneCard>
  )
}
