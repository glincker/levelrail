import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  CountdownGlassIcon,
  EyeOffGlassIcon,
  RotateCcwGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { toast } from '@/components/ui/toast'
import { CleanUpDockerDialog } from '../CleanUpDockerDialog'
import { ApplyFindingDialog } from './ApplyFindingDialog'
import { EvidenceChart } from './EvidenceChart'
import {
  useFindingState,
  type FindingStateVerb,
} from '../../queries/resourceAdvisor'
import {
  SEVERITY_BADGE,
  formatActionValue,
  isKnownKind,
} from '../../lib/resourceAdvisor'
import { formatBytes, formatNanoCpus } from '../../lib/format'
import type { AdvisorFinding } from '../../types/resourceAdvisor'

function useFindingTitle(f: AdvisorFinding): string {
  const { t } = useTranslation('resourceAdvisor')
  if (!isKnownKind(f.kind)) return f.title
  return t(`kinds.${f.kind}`, { ...f.params, defaultValue: f.title })
}

function EvidenceNumbers({ f }: { f: AdvisorFinding }) {
  const { t } = useTranslation('resourceAdvisor')
  const ev = f.evidence
  const fmt = (v?: number) =>
    v === undefined
      ? ''
      : ev.unit === 'nano_cpus'
        ? formatNanoCpus(v)
        : ev.unit === 'bytes'
          ? formatBytes(v)
          : String(v)
  const cells: [string, string][] = []
  if (ev.p95) cells.push([t('finding.p95'), fmt(ev.p95)])
  if (ev.p99) cells.push([t('finding.p99'), fmt(ev.p99)])
  if (ev.max) cells.push([t('finding.peak'), fmt(ev.max)])
  if (ev.unit === 'bytes' || ev.unit === 'nano_cpus') {
    cells.push([
      t('finding.limit'),
      ev.limit ? fmt(ev.limit) : t('finding.noLimit'),
    ])
  }
  return (
    <div className="space-y-2">
      {ev.window ? (
        <p className="text-xs text-muted-foreground">
          {t('finding.window', { window: ev.window, count: ev.samples })}
        </p>
      ) : null}
      {cells.length > 0 ? (
        <dl className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs sm:grid-cols-4">
          {cells.map(([k, v]) => (
            <div key={k}>
              <dt className="text-muted-foreground">{k}</dt>
              <dd className="font-medium tabular-nums text-foreground">{v}</dd>
            </div>
          ))}
        </dl>
      ) : null}
    </div>
  )
}

function SettingsTable({ settings }: { settings: Record<string, string> }) {
  const { t } = useTranslation('resourceAdvisor')
  const keys = Object.keys(settings).sort()
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-foreground">
        {t('finding.settings')}
      </p>
      <dl className="grid grid-cols-1 gap-1 font-mono text-xs sm:grid-cols-2">
        {keys.map((k) => (
          <div
            key={k}
            className="flex justify-between gap-2 rounded bg-muted px-2 py-1"
          >
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="text-right text-foreground">{settings[k]}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

export function FindingCard({ finding: f }: { finding: AdvisorFinding }) {
  const { t } = useTranslation('resourceAdvisor')
  const title = useFindingTitle(f)
  const [applyOpen, setApplyOpen] = useState(false)
  const state = useFindingState()
  const saving =
    (f.impact.saving_memory_bytes ?? 0) + (f.impact.saving_disk_bytes ?? 0)
  const to = f.action ? formatActionValue(f.action.type, f.action.value) : null
  const from = f.action
    ? formatActionValue(f.action.type, f.action.current)
    : null
  const change_ =
    f.appliable && to
      ? from
        ? t('finding.change', { from, to })
        : t('finding.suggestValue', { to })
      : null

  function change(verb: FindingStateVerb) {
    state.mutate(
      { id: f.id, verb, duration: verb === 'snooze' ? '7d' : undefined },
      {
        onError: (err) =>
          toast.add({
            type: 'error',
            title: t('finding.stateError', { message: err.message }),
          }),
      },
    )
  }

  return (
    <article
      className="space-y-3 rounded-lg border border-border p-4"
      data-testid="advisor-finding"
    >
      <header className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0 space-y-1">
          <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            {title}
          </h3>
          <p className="text-sm text-muted-foreground">{f.why}</p>
        </div>
        <Badge variant={SEVERITY_BADGE[f.severity]}>
          {t(`severity.${f.severity}`)}
        </Badge>
      </header>

      <div className="flex flex-wrap gap-2 text-xs">
        {change_ ? <Badge variant="outline">{change_}</Badge> : null}
        {saving > 0 ? (
          <Badge variant="success">
            {t('finding.saving', { amount: formatBytes(saving) })}
          </Badge>
        ) : null}
        {f.impact.exhaustion_at ? (
          <Badge variant="warning">
            {t('finding.exhaustion', {
              date: new Date(f.impact.exhaustion_at).toLocaleDateString(),
            })}
          </Badge>
        ) : null}
        {f.state === 'dismissed' ? (
          <Badge variant="muted">{t('finding.dismissed')}</Badge>
        ) : null}
        {f.state === 'snoozed' && f.snoozed_until ? (
          <Badge variant="muted">
            {t('finding.snoozedUntil', {
              date: new Date(f.snoozed_until).toLocaleDateString(),
            })}
          </Badge>
        ) : null}
      </div>

      {f.action?.settings ? (
        <SettingsTable settings={f.action.settings} />
      ) : null}
      {f.action?.instructions ? (
        <p className="text-xs text-muted-foreground">
          <span className="font-medium text-foreground">
            {t('finding.how')}:{' '}
          </span>
          {f.action.instructions}
        </p>
      ) : null}
      {f.impact.risk ? (
        <p className="text-xs text-muted-foreground">
          <span className="font-medium text-foreground">
            {t('finding.risk')}:{' '}
          </span>
          {f.impact.risk}
        </p>
      ) : null}

      {f.evidence.samples > 0 ? (
        <Collapsible>
          <CollapsibleTrigger className="group flex items-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground">
            <ChevronDownGlassIcon className="size-3.5 transition-transform group-data-[panel-open]:rotate-180" />
            {t('finding.evidence')}
          </CollapsibleTrigger>
          <CollapsiblePanel className="space-y-3 pt-3">
            <EvidenceChart
              evidence={f.evidence}
              suggested={
                f.action?.value &&
                f.appliable &&
                f.action.type !== 'enable_sleep'
                  ? f.action.value
                  : undefined
              }
            />
            <EvidenceNumbers f={f} />
          </CollapsiblePanel>
        </Collapsible>
      ) : null}

      <footer className="flex flex-wrap items-center gap-2">
        {f.can_apply && !f.state ? (
          <Button size="sm" onClick={() => setApplyOpen(true)}>
            {t('finding.apply')}
          </Button>
        ) : null}
        {f.action?.type === 'prune_docker' ? <CleanUpDockerDialog /> : null}
        {f.state ? (
          <Button
            size="sm"
            variant="outline"
            onClick={() => change('restore')}
            disabled={state.isPending}
          >
            <RotateCcwGlassIcon size="1.2em" />
            {t('finding.restore')}
          </Button>
        ) : (
          <>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => change('snooze')}
              disabled={state.isPending}
            >
              <CountdownGlassIcon size="1.2em" />
              {t('finding.snooze')}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => change('dismiss')}
              disabled={state.isPending}
            >
              <EyeOffGlassIcon size="1.2em" />
              {t('finding.dismiss')}
            </Button>
          </>
        )}
        {f.appliable && !f.can_apply ? (
          <span className="text-xs text-muted-foreground">
            {t('finding.notAllowed', { name: f.resource.name })}
          </span>
        ) : null}
      </footer>
      {f.can_apply ? (
        <ApplyFindingDialog
          finding={f}
          open={applyOpen}
          onOpenChange={setApplyOpen}
        />
      ) : null}
    </article>
  )
}
