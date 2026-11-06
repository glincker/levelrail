import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { RoleGate } from '@/components/ui/RoleGate'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatSize } from '../../lib/format'
import {
  useJanitorPlan,
  useJanitorPlanHistory,
  useJanitorPlanRun,
  type JanitorPlanReport,
} from '../../queries/janitorPlan'
import {
  buildPlan,
  defaultSelection,
  groupBytes,
  rowKey,
  selectionTotals,
  stageSelected,
  toAllowlist,
  toggleRow,
  toggleStage,
  type PlanGroup,
  type PlanRow,
} from './cleanupPlan'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface Outcome {
  kind: 'preview' | 'applied'
  report: JanitorPlanReport
}

interface Chosen {
  planId: string
  set: Set<string>
}

export function CleanupPlanDrawer({ open, onOpenChange }: Props) {
  const { t } = useTranslation('storageCleanup')
  const plan = useJanitorPlan(open)
  const run = useJanitorPlanRun()
  const [chosen, setChosen] = useState<Chosen | null>(null)
  const [confirming, setConfirming] = useState(false)
  const [outcome, setOutcome] = useState<Outcome | null>(null)

  const groups = useMemo(
    () => (plan.data ? buildPlan(plan.data) : []),
    [plan.data],
  )
  const selection =
    plan.data && chosen && chosen.planId === plan.data.id
      ? chosen.set
      : defaultSelection(groups)
  const totals = selectionTotals(groups, selection)
  const hasRows = groups.some((g) => g.rows.length > 0)

  function change(next: Set<string>) {
    if (!plan.data) return
    setChosen({ planId: plan.data.id, set: next })
    setConfirming(false)
    setOutcome((o) => (o?.kind === 'preview' ? null : o))
  }

  function start(dryRun: boolean) {
    setConfirming(false)
    run.mutate(
      { dryRun, items: toAllowlist(groups, selection) },
      {
        onSuccess: (report) =>
          setOutcome({ kind: dryRun ? 'preview' : 'applied', report }),
      },
    )
  }

  const busy = run.isPending
  const nothingPicked = totals.count === 0

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="data-[side=right]:sm:max-w-xl">
        <SheetHeader className="pr-12">
          <SheetTitle>{t('title')}</SheetTitle>
          <SheetDescription>{t('description')}</SheetDescription>
        </SheetHeader>
        <Tabs defaultValue="plan" className="min-h-0 flex-1 px-4">
          <TabsList variant="line" aria-label={t('tabs.label')}>
            <TabsTrigger value="plan">{t('tabs.plan')}</TabsTrigger>
            <TabsTrigger value="history">{t('tabs.history')}</TabsTrigger>
          </TabsList>
          <TabsContent
            value="plan"
            className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto pb-2"
          >
            {plan.isPending ? (
              <div role="status" aria-label={t('loading')}>
                <ListSkeleton rows={4} />
              </div>
            ) : plan.isError ? (
              <Alert variant="destructive">
                <AlertDescription className="flex items-center justify-between gap-2">
                  <span>{t('error')}</span>
                  <Button
                    size="xs"
                    variant="outline"
                    onClick={() => void plan.refetch()}
                  >
                    {t('retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            ) : !hasRows ? (
              <p className="py-6 text-center text-sm text-muted-foreground">
                {t('empty')}
              </p>
            ) : (
              groups.map((g) => (
                <StageGroup
                  key={g.stage}
                  group={g}
                  selection={selection}
                  onChange={change}
                />
              ))
            )}
            {outcome && <OutcomePanel outcome={outcome} />}
            {run.isError && (
              <Alert variant="destructive">
                <AlertDescription>{run.error.message}</AlertDescription>
              </Alert>
            )}
          </TabsContent>
          <TabsContent value="history" className="overflow-y-auto pb-2">
            <HistoryList enabled={open} />
          </TabsContent>
        </Tabs>
        <SheetFooter className="sticky bottom-0 gap-2 border-t bg-popover">
          {confirming ? (
            <div
              role="alertdialog"
              aria-label={t('apply')}
              className="flex flex-col gap-2"
            >
              <p className="text-sm font-medium">
                {t('confirm.line', {
                  size: formatSize(totals.bytes),
                  count: totals.count,
                })}
              </p>
              <div className="flex gap-2">
                <Button
                  variant="destructive"
                  size="sm"
                  disabled={busy}
                  onClick={() => start(false)}
                >
                  {busy ? t('applying') : t('confirm.yes')}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setConfirming(false)}
                >
                  {t('confirm.cancel')}
                </Button>
              </div>
            </div>
          ) : (
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-sm text-muted-foreground" aria-live="polite">
                {nothingPicked
                  ? t('none')
                  : t('selected', {
                      size: formatSize(totals.bytes),
                      count: totals.count,
                    })}
              </p>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy || nothingPicked}
                  onClick={() => start(true)}
                >
                  {busy && run.variables?.dryRun
                    ? t('previewing')
                    : t('preview')}
                </Button>
                <RoleGate ability="root">
                  <Button
                    size="sm"
                    disabled={busy || nothingPicked}
                    onClick={() => setConfirming(true)}
                  >
                    {t('apply')}
                  </Button>
                </RoleGate>
              </div>
            </div>
          )}
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function StageGroup({
  group,
  selection,
  onChange,
}: {
  group: PlanGroup
  selection: Set<string>
  onChange: (next: Set<string>) => void
}) {
  const { t } = useTranslation('storageCleanup')
  const stageName = t(`stages.${group.stage}`)
  const headingId = `cleanup-stage-${group.stage}`
  const hasSelectable = group.rows.some((r) => r.selectable)
  return (
    <section aria-labelledby={headingId} className="rounded-lg border">
      <header className="flex items-center gap-2 border-b px-3 py-2">
        {group.whole && hasSelectable && (
          <Checkbox
            aria-label={t('selectStage', { stage: stageName })}
            checked={stageSelected(group, selection)}
            onCheckedChange={() => onChange(toggleStage(selection, group))}
          />
        )}
        <h3 id={headingId} className="text-sm font-medium">
          {stageName}
        </h3>
        {group.whole && hasSelectable && (
          <span className="text-xs text-muted-foreground">
            {t('wholeStage')}
          </span>
        )}
        <span className="ml-auto text-xs text-muted-foreground tabular-nums">
          {formatSize(groupBytes(group))}
        </span>
      </header>
      {group.note && (
        <p className="px-3 py-2 text-xs text-muted-foreground">
          {t('stageNote', { reason: group.note })}
        </p>
      )}
      <ul className="divide-y">
        {group.rows.map((r) => (
          <PlanRowItem
            key={rowKey(r.stage, r.name)}
            row={r}
            whole={group.whole}
            checked={selection.has(rowKey(r.stage, r.name))}
            onToggle={() => onChange(toggleRow(selection, r.stage, r.name))}
          />
        ))}
      </ul>
    </section>
  )
}

function PlanRowItem({
  row,
  whole,
  checked,
  onToggle,
}: {
  row: PlanRow
  whole: boolean
  checked: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation('storageCleanup')
  const reason = row.lock === 'database' ? t('locked.database') : row.detail
  return (
    <li className="flex items-start gap-2 px-3 py-2">
      <span className="flex h-5 w-4 shrink-0 items-center">
        {row.lock ? (
          <LockGlassIcon
            className="size-3.5 text-muted-foreground"
            aria-label={t('locked.label')}
          />
        ) : (
          !whole && (
            <Checkbox
              aria-label={t('selectRow', { name: row.name })}
              checked={checked}
              onCheckedChange={onToggle}
            />
          )
        )}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-sm" title={row.name}>
          {row.name}
        </p>
        {reason && <p className="text-xs text-muted-foreground">{reason}</p>}
      </div>
      <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
        {formatSize(row.sizeBytes)}
      </span>
    </li>
  )
}

function OutcomePanel({ outcome }: { outcome: Outcome }) {
  const { t } = useTranslation('storageCleanup')
  const { report } = outcome
  const ignored = report.ignored_items ?? []
  const size = formatSize(report.reclaimed_bytes)
  return (
    <div role="status" className="rounded-lg border bg-muted/40 px-3 py-2">
      <p className="text-sm font-medium">
        {outcome.kind === 'applied'
          ? t('result.applied', { size })
          : t('result.previewed', { size })}
      </p>
      {report.error && (
        <p className="text-xs text-destructive">
          {t('result.failed', { error: report.error })}
        </p>
      )}
      {ignored.length > 0 && (
        <div className="mt-2">
          <p className="text-xs font-medium">{t('result.ignoredTitle')}</p>
          <ul className="text-xs text-muted-foreground">
            {ignored.map((i) => (
              <li key={rowKey(i.stage, i.name)}>
                {t('result.ignoredRow', {
                  name: i.name,
                  stage: t(`stages.${i.stage}`),
                  reason: i.reason,
                })}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}

function HistoryList({ enabled }: { enabled: boolean }) {
  const { t } = useTranslation('storageCleanup')
  const history = useJanitorPlanHistory(enabled)
  if (history.isPending) {
    return (
      <div role="status" aria-label={t('history.loading')}>
        <ListSkeleton rows={4} />
      </div>
    )
  }
  if (history.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{t('history.error')}</AlertDescription>
      </Alert>
    )
  }
  if (history.data.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        {t('history.empty')}
      </p>
    )
  }
  return (
    <ul className="divide-y rounded-lg border">
      {history.data.map((r) => (
        <li
          key={r.id}
          className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2"
        >
          <time
            dateTime={r.started_at}
            className="text-sm tabular-nums"
            title={t('history.time')}
          >
            {new Date(r.started_at).toLocaleString()}
          </time>
          <span className="text-xs text-muted-foreground">
            {r.actor
              ? `${r.actor.name} (${t(`history.kind.${r.actor.kind}`)})`
              : t(`history.trigger.${r.trigger}`)}
          </span>
          <span className="ml-auto text-sm tabular-nums">
            {formatSize(r.reclaimed_bytes)}
          </span>
          <Badge variant={r.dry_run ? 'outline' : 'muted'}>
            {r.dry_run ? t('history.dryRun') : t('history.applied')}
          </Badge>
        </li>
      ))}
    </ul>
  )
}
