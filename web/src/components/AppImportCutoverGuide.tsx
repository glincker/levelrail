import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  cutoverSettled,
  useConfirmCutoverDns,
  useCutoverPlan,
  useCutoverRuns,
  useRollbackCutover,
  useStartCutover,
  type CutoverCheck,
  type CutoverRun,
  type CutoverState,
} from '../queries/appImportCutover'

const checkVariant = {
  pass: 'success',
  warn: 'warning',
  block: 'destructive',
} as const

const stateVariant: Record<
  CutoverState,
  'success' | 'warning' | 'destructive' | 'default'
> = {
  planning: 'default',
  ready: 'success',
  starting: 'warning',
  verifying: 'warning',
  switching: 'warning',
  live: 'success',
  rolled_back: 'default',
  failed: 'destructive',
}

function CheckRow({ c }: { c: CutoverCheck }) {
  const { t } = useTranslation('migration')
  const copy = c.fix?.action?.kind === 'copy' ? c.fix.action : undefined
  return (
    <li className="flex flex-wrap items-start gap-2 text-sm">
      <Badge variant={checkVariant[c.status]}>
        {t(`appImport.cutover.guide.status.${c.status}`)}
      </Badge>
      <span className="flex-1">
        <span className="font-medium">{c.title}</span>
        {c.detail ? (
          <span className="block text-muted-foreground">{c.detail}</span>
        ) : null}
        {c.status !== 'pass' && c.fix ? (
          <span className="block text-muted-foreground">{c.fix.summary}</span>
        ) : null}
      </span>
      {copy?.value ? (
        <Button
          size="sm"
          variant="outline"
          onClick={() => void navigator.clipboard.writeText(copy.value ?? '')}
        >
          {t('appImport.cutover.guide.copyRecord')}
        </Button>
      ) : null}
    </li>
  )
}

function RunPanel({
  run,
  sessionId,
  item,
}: {
  run: CutoverRun
  sessionId: string
  item: string
}) {
  const { t } = useTranslation('migration')
  const rollback = useRollbackCutover(sessionId, item)
  const confirm = useConfirmCutoverDns(sessionId, item)
  const manual = run.awaiting
    ? run.domains.filter((d) => d.method === 'manual' && d.manual)
    : []
  return (
    <div className="space-y-2 rounded-md border p-3" aria-live="polite">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">
          {t(`appImport.cutover.guide.mode.${run.mode}`)}
        </span>
        <Badge variant={stateVariant[run.state]}>
          {t(`appImport.cutover.guide.state.${run.state}`)}
        </Badge>
        <div className="ml-auto flex gap-2">
          {run.awaiting ? (
            <Button
              size="sm"
              disabled={confirm.isPending}
              onClick={() =>
                confirm.mutate(
                  { run: run.id },
                  {
                    onError: (e) =>
                      toast.add({ title: e.message, type: 'error' }),
                  },
                )
              }
            >
              {t('appImport.cutover.guide.confirmDns')}
            </Button>
          ) : null}
          {run.rollbackable ? (
            <Button
              size="sm"
              variant="outline"
              disabled={rollback.isPending}
              onClick={() =>
                rollback.mutate(
                  { run: run.id },
                  {
                    onError: (e) =>
                      toast.add({ title: e.message, type: 'error' }),
                  },
                )
              }
            >
              {t('appImport.cutover.guide.rollback')}
            </Button>
          ) : null}
        </div>
      </div>
      {run.error ? (
        <Alert variant={run.state === 'live' ? 'default' : 'destructive'}>
          <AlertDescription>{run.error}</AlertDescription>
        </Alert>
      ) : null}
      {manual.map((d) => (
        <div
          key={d.domain}
          className="rounded-md bg-muted p-2 font-mono text-xs"
        >
          {t('appImport.cutover.guide.manualRecord', {
            domain: d.domain,
            type: d.manual?.type,
            value: d.manual?.value,
          })}
        </div>
      ))}
      <ol className="space-y-1 text-sm">
        {run.steps.map((s, i) => (
          <li key={`${s.name}-${s.domain ?? ''}-${i}`} className="flex gap-2">
            <Badge
              variant={
                s.state === 'failed'
                  ? 'destructive'
                  : s.state === 'done'
                    ? 'success'
                    : 'default'
              }
            >
              {t(`appImport.cutover.guide.step.${s.name}`, {
                defaultValue: s.name,
              })}
            </Badge>
            <span className="flex-1 text-muted-foreground">
              {s.domain ? `${s.domain}: ` : ''}
              {s.detail}
            </span>
            <span className="text-xs text-muted-foreground">
              {s.duration_ms} ms
            </span>
          </li>
        ))}
      </ol>
      {run.state === 'live' ? (
        <p className="text-xs text-muted-foreground">
          {t('appImport.cutover.guide.liveHint')}
        </p>
      ) : null}
    </div>
  )
}

export function AppImportCutoverGuide({
  sessionId,
  item,
  app,
}: {
  sessionId: string
  item: string
  app: string
}) {
  const { t } = useTranslation('migration')
  const [open, setOpen] = useState(false)
  const [typed, setTyped] = useState('')
  const [accept, setAccept] = useState(false)
  const plan = useCutoverPlan(sessionId, item, open)
  const runs = useCutoverRuns(sessionId, item, open)
  const start = useStartCutover(sessionId, item)
  const latest = runs.data?.[0]
  const busy = start.isPending || !cutoverSettled(latest)
  const verdict = plan.data?.verdict
  const blocked = verdict === 'blocked'
  const needsAccept = verdict === 'warnings'
  const canSwitch =
    !busy &&
    !blocked &&
    typed.trim() === app &&
    (!needsAccept || accept) &&
    latest?.state !== 'live'

  function launch(mode: 'dry_run' | 'switch') {
    start.mutate(
      { mode, confirm: typed.trim(), acceptWarnings: accept },
      {
        onError: (e) => toast.add({ title: e.message, type: 'error' }),
        onSuccess: () => setTyped(''),
      },
    )
  }

  if (!open) {
    return (
      <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
        {t('appImport.cutover.guide.open')}
      </Button>
    )
  }
  return (
    <section
      className="space-y-3 rounded-md border bg-muted/20 p-3"
      aria-label={t('appImport.cutover.guide.title', { app })}
    >
      <div className="flex flex-wrap items-center gap-2">
        <h4 className="text-sm font-semibold">
          {t('appImport.cutover.guide.title', { app })}
        </h4>
        {verdict ? (
          <Badge
            variant={
              blocked ? 'destructive' : needsAccept ? 'warning' : 'success'
            }
          >
            {t(`appImport.cutover.guide.verdict.${verdict}`)}
          </Badge>
        ) : null}
        <div className="ml-auto flex gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={plan.isFetching}
            onClick={() => void plan.refetch()}
          >
            {t('appImport.cutover.guide.recheck')}
          </Button>
          <Button size="sm" variant="ghost" onClick={() => setOpen(false)}>
            {t('appImport.cutover.guide.close')}
          </Button>
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        {t('appImport.cutover.guide.safety')}
      </p>
      {plan.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{plan.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {plan.data ? (
        <>
          <ul className="space-y-2">
            {plan.data.checks.map((c, i) => (
              <CheckRow key={`${c.id}-${i}`} c={c} />
            ))}
          </ul>
          <ul className="space-y-1 text-xs text-muted-foreground">
            {plan.data.domains.map((d) => (
              <li key={d.domain}>
                {t('appImport.cutover.guide.domainLine', {
                  domain: d.domain,
                  method: t(`appImport.cutover.guide.method.${d.method}`),
                  current: d.current?.join(', ') || '-',
                })}
              </li>
            ))}
          </ul>
        </>
      ) : null}
      <div className="flex flex-wrap items-end gap-3 border-t pt-3">
        <Button
          variant="outline"
          disabled={busy || blocked}
          onClick={() => launch('dry_run')}
        >
          {t('appImport.cutover.guide.dryRun')}
        </Button>
        <div className="min-w-48 space-y-1.5">
          <Label htmlFor={`cutover-confirm-${item}`}>
            {t('appImport.cutover.guide.typeToConfirm', { app })}
          </Label>
          <Input
            id={`cutover-confirm-${item}`}
            autoComplete="off"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            placeholder={app}
          />
        </div>
        {needsAccept ? (
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={accept}
              onCheckedChange={(v) => setAccept(v === true)}
            />
            {t('appImport.cutover.guide.acceptWarnings')}
          </label>
        ) : null}
        <Button disabled={!canSwitch} onClick={() => launch('switch')}>
          {t('appImport.cutover.guide.switch')}
        </Button>
      </div>
      {latest ? (
        <RunPanel run={latest} sessionId={sessionId} item={item} />
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('appImport.cutover.guide.noRuns')}
        </p>
      )}
    </section>
  )
}
