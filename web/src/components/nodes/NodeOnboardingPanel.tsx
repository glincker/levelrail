import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { useNodeByName, useNodeOnboarding } from '../../queries/nodeOnboarding'
import type {
  PreflightCheck,
  PreflightStatus,
} from '../../types/nodeOnboarding'
import { CommandBlock } from './CommandBlock'

type StepState = 'done' | 'active' | 'pending' | 'attention'

const STEP_LABEL = {
  token: 'onboarding.steps.token',
  contacted: 'onboarding.steps.contacted',
  connected: 'onboarding.steps.connected',
  preflight: 'onboarding.steps.preflight',
} as const

const STATUS_ICON: Record<PreflightStatus, typeof CircleCheckGlassIcon> = {
  pass: CircleCheckGlassIcon,
  warn: TriangleAlertGlassIcon,
  fail: CircleXGlassIcon,
  unknown: CircleMinusGlassIcon,
}

const STATUS_TONE: Record<PreflightStatus, string> = {
  pass: 'text-tone-success',
  warn: 'text-tone-warning',
  fail: 'text-tone-danger',
  unknown: 'text-muted-foreground',
}

const STATUS_BADGE: Record<
  PreflightStatus,
  'success' | 'warning' | 'destructive' | 'muted'
> = {
  pass: 'success',
  warn: 'warning',
  fail: 'destructive',
  unknown: 'muted',
}

function StepIcon({ state }: { state: StepState }) {
  if (state === 'done') {
    return <CircleCheckGlassIcon className="size-5 text-tone-success" />
  }
  if (state === 'active') {
    return (
      <LoaderCircleGlassIcon className="size-5 animate-spin text-muted-foreground motion-reduce:animate-none" />
    )
  }
  if (state === 'attention') {
    return <TriangleAlertGlassIcon className="size-5 text-tone-warning" />
  }
  return <CircleGlassIcon className="size-5 text-muted-foreground" />
}

// Live progress for a node that was just given a join command: contacted,
// connected over mTLS, then its preflight checks with a fix for each problem.
export function NodeOnboardingPanel({ nodeName }: { nodeName: string }) {
  const { t } = useTranslation('nodes')
  const node = useNodeByName(nodeName, true)
  const nodeId = node.data?.id
  const onboarding = useNodeOnboarding(nodeId)
  const ob = onboarding.data

  const contacted = Boolean(nodeId)
  const connected = ob
    ? ob.stage === 'connected' ||
      ob.stage === 'ready' ||
      ob.stage === 'needs_attention'
    : false
  const reported = Boolean(ob?.reported)
  const needsAttention = ob?.stage === 'needs_attention'

  const steps: { key: keyof typeof STEP_LABEL; state: StepState }[] = [
    { key: 'token', state: 'done' },
    { key: 'contacted', state: contacted ? 'done' : 'active' },
    {
      key: 'connected',
      state: connected ? 'done' : contacted ? 'active' : 'pending',
    },
    {
      key: 'preflight',
      state: reported
        ? needsAttention
          ? 'attention'
          : 'done'
        : connected
          ? 'active'
          : 'pending',
    },
  ]

  return (
    <section className="space-y-3" aria-labelledby="node-onboarding-title">
      <h3
        id="node-onboarding-title"
        className="text-sm font-medium text-foreground"
      >
        {t('onboarding.title', { name: nodeName })}
      </h3>
      <ol className="space-y-1.5" aria-live="polite">
        {steps.map((s) => (
          <li key={s.key} className="flex items-center gap-2 text-sm">
            <StepIcon state={s.state} />
            <span
              className={
                s.state === 'pending'
                  ? 'text-muted-foreground'
                  : 'text-foreground'
              }
            >
              {t(STEP_LABEL[s.key])}
            </span>
          </li>
        ))}
      </ol>

      {ob?.reported ? (
        <div className="space-y-3">
          <DomainLine
            detected={ob.detected}
            applied={ob.domain}
            stale={ob.stale}
          />
          <ul className="space-y-2">
            {ob.checks.map((c) => (
              <CheckRow key={c.id} check={c} />
            ))}
          </ul>
          <div className="flex items-center justify-between gap-2">
            <p className="text-xs text-muted-foreground">
              {t('onboarding.summary', {
                pass: ob.summary.pass,
                warn: ob.summary.warn,
                fail: ob.summary.fail,
                unknown: ob.summary.unknown,
              })}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={onboarding.isFetching}
              onClick={() => void onboarding.refetch()}
            >
              {t('onboarding.recheck')}
            </Button>
          </div>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          {contacted ? t('onboarding.waitingFacts') : t('onboarding.waiting')}
        </p>
      )}
    </section>
  )
}

function DomainLine({
  detected,
  applied,
  stale,
}: {
  detected:
    | { provider?: string; region?: string; zone?: string; source?: string }
    | undefined
  applied: { provider?: string; region?: string; zone?: string }
  stale: boolean
}) {
  const { t } = useTranslation('nodes')
  const none = t('onboarding.none')
  const found =
    detected && (detected.provider || detected.region || detected.zone)
  return (
    <div className="space-y-1 rounded-lg border border-border bg-muted/40 p-2.5 text-xs">
      <p className="text-foreground">
        {found
          ? t('onboarding.detected', {
              provider: detected.provider || none,
              region: detected.region || none,
              zone: detected.zone || none,
              source:
                detected.source === 'env'
                  ? t('onboarding.source.env')
                  : t('onboarding.source.metadata'),
            })
          : t('onboarding.notDetected')}
      </p>
      <p className="text-muted-foreground">
        {t('onboarding.applied', {
          provider: applied.provider || none,
          region: applied.region || none,
          zone: applied.zone || none,
        })}
      </p>
      {stale ? (
        <p className="text-tone-warning">{t('onboarding.stale')}</p>
      ) : null}
    </div>
  )
}

function CheckRow({ check }: { check: PreflightCheck }) {
  const { t } = useTranslation('nodes')
  const Icon = STATUS_ICON[check.status]
  return (
    <li className="space-y-1.5 rounded-lg border border-border p-2.5">
      <div className="flex items-center gap-2">
        <Icon className={`size-4 shrink-0 ${STATUS_TONE[check.status]}`} />
        <span className="text-sm font-medium text-foreground">
          {t(`onboarding.checks.${check.id}`, { defaultValue: check.id })}
        </span>
        <Badge variant={STATUS_BADGE[check.status]} className="ml-auto">
          {t(`onboarding.status.${check.status}`)}
        </Badge>
      </div>
      <p className="text-xs text-muted-foreground">{check.detail}</p>
      {check.fix && check.status !== 'pass' ? (
        <p className="text-xs text-foreground">
          <span className="font-medium">{t('onboarding.fix')}: </span>
          {check.fix}
        </p>
      ) : null}
      {check.fix_command && check.status !== 'pass' ? (
        <Collapsible>
          <CollapsibleTrigger className="text-xs text-muted-foreground underline-offset-2 hover:underline">
            {t('onboarding.fixViaCli')}
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <CommandBlock
              id={`node-fix-${check.id}`}
              label={t('onboarding.runOnNode')}
              value={check.fix_command}
            />
          </CollapsiblePanel>
        </Collapsible>
      ) : null}
    </li>
  )
}
