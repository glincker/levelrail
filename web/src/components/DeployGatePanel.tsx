import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  CountdownGlassIcon,
  EyeGlassIcon,
  LogOutGlassIcon,
  PinGlassIcon,
  RotateCcwGlassIcon,
  ShieldCheckGlassIcon,
  SignpostGlassIcon,
} from '@thesvg/ui-react'
import type { ReactNode } from 'react'
import { Timeline, type TimelineItem } from '@/components/kit'
import type { Tone } from '@/components/kit/tone'
import { StatusBadge } from '@/components/ui/status-badge'
import { useDeployGate } from '../queries/deployGate'
import { WatchActions, WatchSummary } from './WatchControls'
import type {
  DeployGate,
  GateEvent,
  GatePhase,
  GateStage,
} from '../types/deployGate'

// Crash retries add steps on every pass; older ones stay in the API.
const MAX_VISIBLE_EVENTS = 40

const PHASE_ICON: Record<GatePhase, ReactNode> = {
  readiness: <CountdownGlassIcon size="1.2em" aria-hidden="true" />,
  stability: (
    <ShieldCheckGlassIcon
      className="text-tone-success"
      size="1.2em"
      aria-hidden="true"
    />
  ),
  cutover: <SignpostGlassIcon size="1.2em" aria-hidden="true" />,
  drain: <LogOutGlassIcon size="1.2em" aria-hidden="true" />,
  watch: <EyeGlassIcon size="1.2em" aria-hidden="true" />,
  rollback: <RotateCcwGlassIcon size="1.2em" aria-hidden="true" />,
  pin: <PinGlassIcon size="1.2em" aria-hidden="true" />,
}

const STAGE_TONE: Record<GateStage, Tone> = {
  pending: 'neutral',
  gating: 'info',
  gate_failed: 'danger',
  watching: 'info',
  cleared: 'success',
  watch_failed: 'danger',
  rolled_back: 'warning',
  watch_stopped: 'neutral',
}

function eventTone(e: GateEvent): Tone {
  if (e.status === 'failed') return 'danger'
  if (e.phase === 'rollback') return 'warning'
  return e.status === 'passed' ? 'success' : 'neutral'
}

/** DeployGatePanel shows how a release earned traffic and how its post-deploy watch went. */
export function DeployGatePanel({
  appName,
  deployId,
}: {
  appName: string
  deployId: string
}) {
  const { t } = useTranslation('deploySafety')
  const { data: gate, isPending, isError } = useDeployGate(appName, deployId)

  if (isError) {
    return (
      <p className="text-sm text-muted-foreground">{t('gate.unavailable')}</p>
    )
  }
  if (isPending) {
    return <Timeline items={[]} loading />
  }
  return (
    <div className="space-y-3">
      <GateView gate={gate} />
      {gate.watch.state === 'watching' ? (
        <WatchActions appName={appName} />
      ) : null}
    </div>
  )
}

export function GateView({ gate }: { gate: DeployGate }) {
  const { t } = useTranslation('deploySafety')
  const hidden = Math.max(0, gate.events.length - MAX_VISIBLE_EVENTS)
  const items: TimelineItem[] = gate.events.slice(hidden).map((e, i) => ({
    id: `${e.at}-${i}`,
    at: e.at,
    icon: PHASE_ICON[e.phase] ?? (
      <CircleCheckGlassIcon
        className="text-tone-success"
        size="1.2em"
        aria-hidden="true"
      />
    ),
    tone: eventTone(e),
    title: t(`phase.${e.phase}`, { defaultValue: e.phase }),
    detail: e.reason ? `${e.message} (${e.reason})` : e.message,
  }))
  const s = gate.settings
  const failed = gate.stage === 'gate_failed' || gate.stage === 'watch_failed'

  return (
    <section className="space-y-3" aria-label={t('gate.title')}>
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {t('gate.title')}
        </h3>
        <StatusBadge
          tone={STAGE_TONE[gate.stage]}
          label={t(`stage.${gate.stage}`)}
          icon={failed ? CircleXGlassIcon : ShieldCheckGlassIcon}
        />
      </div>
      <p className="text-sm text-muted-foreground">
        {t('gate.rules', {
          passes: s.success_threshold,
          stable: s.stable_window,
          drain: s.drain_timeout,
          watch: s.watch_window,
        })}
      </p>
      <WatchSummary watch={gate.watch} />
      {gate.rollback_pin ? (
        <p className="text-sm text-muted-foreground">
          {t('gate.pinned', { ref: gate.rollback_pin.pin_ref })}
        </p>
      ) : null}
      {hidden > 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('gate.hidden', { count: hidden })}
        </p>
      ) : null}
      <Timeline items={items} emptyLabel={t('gate.empty')} />
    </section>
  )
}
