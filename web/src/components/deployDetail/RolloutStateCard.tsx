import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { RotateCcwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { RelativeTime } from '../kit'
import { formatCountdown } from '../../lib/signInRequest'
import {
  useRolloutState,
  type RolloutTraffic,
} from '../../queries/rolloutState'

const TRAFFIC_VARIANT: Record<RolloutTraffic, 'muted' | 'success' | 'warning'> =
  {
    held: 'muted',
    flipped: 'success',
    reverted: 'warning',
  }

// Counts down locally between polls so the hold timer does not jump.
function useRemaining(seconds: number, active: boolean): number {
  const [left, setLeft] = useState(seconds)
  const [prev, setPrev] = useState(seconds)
  if (prev !== seconds) {
    setPrev(seconds)
    setLeft(seconds)
  }
  useEffect(() => {
    if (!active) return
    const id = window.setInterval(() => {
      setLeft((n) => Math.max(0, n - 1))
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [active])
  return left
}

/** RolloutStateCard shows where the newest release is between the gate and the end of the hold. */
export function RolloutStateCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploySafety')
  const { data, isPending } = useRolloutState(appName)
  const holdActive = data?.hold.state === 'watching'
  const left = useRemaining(data?.hold.remaining_seconds ?? 0, holdActive)

  if (isPending) return <CardSkeleton titleWidth="w-40" rows={1} />
  if (!data || !data.deploy_id) return null

  const canary = data.canary?.active ? data.canary : null

  return (
    <section
      aria-label={t('rolloutState.title')}
      className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border px-4 py-3"
    >
      <RotateCcwGlassIcon
        className="size-5 shrink-0 text-muted-foreground"
        aria-hidden="true"
      />
      <h3 className="text-sm font-semibold text-foreground">
        {t('rolloutState.title')}
      </h3>
      <Badge variant="outline">
        {t(`stage.${data.stage}`, { defaultValue: data.stage })}
      </Badge>
      <Badge variant={TRAFFIC_VARIANT[data.traffic]}>
        {t(`rolloutState.traffic.${data.traffic}`)}
      </Badge>
      {holdActive ? (
        <span className="text-sm text-muted-foreground" aria-live="off">
          {t('rolloutState.holdLeft', { time: formatCountdown(left) })}
        </span>
      ) : null}
      {data.traffic === 'flipped' && data.flipped_at ? (
        <span className="text-xs text-muted-foreground">
          {t('rolloutState.flipped')} <RelativeTime at={data.flipped_at} />
        </span>
      ) : null}
      {data.traffic === 'reverted' && data.rolled_back_at ? (
        <span className="text-xs text-muted-foreground">
          {t('rolloutState.reverted')} <RelativeTime at={data.rolled_back_at} />
        </span>
      ) : null}
      {data.gate_failures > 0 ? (
        <span className="text-xs text-muted-foreground">
          {t('rolloutState.gateFailures', { count: data.gate_failures })}
        </span>
      ) : null}
      {canary ? (
        <Badge variant="info">
          {t('rolloutState.canary', {
            weight: canary.weight,
            stage: canary.progress?.stage ?? canary.stage ?? 0,
            total: canary.progress?.stage_count ?? canary.stages?.length ?? 0,
          })}
        </Badge>
      ) : null}
      {data.failure_reason ? (
        <p className="basis-full text-sm text-tone-danger">
          {data.failure_reason}
        </p>
      ) : null}
      <Link
        to="/apps/$name/deploy-settings"
        params={{ name: appName }}
        className="ml-auto text-xs text-foreground underline-offset-2 hover:underline"
      >
        {t('rolloutState.settings')}
      </Link>
    </section>
  )
}
