import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { RefreshCwGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import {
  IacIssuesError,
  iacSchemaQueryOptions,
  useApplyIac,
  usePlanIac,
} from '../../queries/iac'
import type { IacChange, IacPlan } from '../../queries/iac'
import { IacPlanView, IacResultView } from '../IacPlanView'
import { IacBetaBadge, IacCodeBadge } from './IacCodeBadge'
import { IacTemplateStrip } from './IacTemplateStrip'

const PLAN_DEBOUNCE_MS = 600
const HASH_PREFIX_LEN = 8
const studioName = 'studio.yaml'

export function IacStudio() {
  const { t } = useTranslation('settings')
  const [yaml, setYaml] = useState('')
  const debounced = useDebouncedValue(yaml, PLAN_DEBOUNCE_MS)
  const planIac = usePlanIac()
  const applyIac = useApplyIac()
  const { data: stability } = useQuery(iacSchemaQueryOptions())
  const [planned, setPlanned] = useState<{
    text: string
    plan: IacPlan
  } | null>(null)
  const plan = planned?.text === debounced ? planned.plan : null
  const { mutate: mutatePlan } = planIac
  const { reset: resetApply } = applyIac

  function runPlan(content: string) {
    resetApply()
    mutatePlan(
      { files: [{ name: studioName, content }] },
      { onSuccess: (p) => setPlanned({ text: content, plan: p }) },
    )
  }

  useEffect(() => {
    if (!debounced.trim()) return
    resetApply()
    mutatePlan(
      { files: [{ name: studioName, content: debounced }] },
      { onSuccess: (p) => setPlanned({ text: debounced, plan: p }) },
    )
  }, [debounced, mutatePlan, resetApply])

  const betaKinds = new Set(
    Object.entries(stability ?? {})
      .filter(([, v]) => v === 'beta')
      .map(([k]) => k),
  )
  const pendingCount = plan
    ? plan.summary.create + plan.summary.update + plan.summary.delete
    : 0
  const canApply =
    plan !== null &&
    pendingCount > 0 &&
    plan.summary.error === 0 &&
    !applyIac.isPending &&
    yaml === debounced
  const applyCode = applyIac.error?.code
  const lineCount = Math.max(1, yaml.split('\n').length)

  return (
    <div className="space-y-4">
      <IacTemplateStrip onUse={setYaml} />
      <div className="flex gap-2">
        <pre
          aria-hidden="true"
          className="max-h-96 min-h-48 overflow-hidden rounded-md border bg-muted/40 px-2 py-2 text-right font-mono text-xs leading-[1.625] text-muted-foreground select-none"
        >
          {Array.from({ length: lineCount }, (_, i) => i + 1).join('\n')}
        </pre>
        <Textarea
          aria-label={t('iacStudio.editor')}
          value={yaml}
          onChange={(e) => setYaml(e.target.value)}
          spellCheck={false}
          placeholder={t('iacStudio.placeholder')}
          className="max-h-96 min-h-48 flex-1 font-mono text-xs leading-[1.625]"
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {t('iacStudio.cloudNote')}
      </p>

      <div aria-live="polite" className="space-y-3">
        {planIac.isPending ? (
          <p className="text-sm text-muted-foreground">
            {t('iacPage.planning')}
          </p>
        ) : null}
        {debounced.trim() && planIac.error instanceof IacIssuesError ? (
          <ul className="space-y-1 text-sm">
            {planIac.error.issues.map((i) => (
              <li
                key={`${i.line}:${i.path ?? ''}:${i.message}`}
                className="flex flex-wrap items-center gap-2"
              >
                {i.code ? <IacCodeBadge code={i.code} /> : null}
                <span className="font-mono text-xs text-tone-danger">
                  {i.line > 0 ? `${i.line}: ` : ''}
                  {i.message}
                  {i.path ? ` (${i.path})` : ''}
                </span>
              </li>
            ))}
          </ul>
        ) : debounced.trim() && planIac.error ? (
          <p className="flex flex-wrap items-center gap-2 text-sm text-tone-danger">
            {planIac.error.code ? (
              <IacCodeBadge code={planIac.error.code} />
            ) : null}
            {planIac.error.message}
          </p>
        ) : null}
        {plan ? (
          <>
            <p className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span>{t('iacStudio.planHash')}</span>
              <code className="font-mono">
                {plan.hash.slice(0, HASH_PREFIX_LEN)}
              </code>
              {plan.changes
                .filter((c) => betaKinds.has(c.kind))
                .map((c) => c.kind)
                .filter((k, i, a) => a.indexOf(k) === i)
                .map((k) => (
                  <span key={k} className="flex items-center gap-1">
                    {k} <IacBetaBadge />
                  </span>
                ))}
            </p>
            <IacPlanView plan={plan} />
            <ChangeCodes changes={plan.changes} />
          </>
        ) : null}
      </div>

      {applyCode === 'apply_disabled' ? (
        <p
          role="alert"
          className="flex items-center gap-2 rounded-md border border-tone-warning-border bg-tone-warning-soft p-2 text-sm text-tone-warning"
        >
          <TriangleAlertGlassIcon size="1.4em" aria-hidden="true" />
          {t('iacStudio.applyDisabled')}
        </p>
      ) : applyIac.error ? (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-tone-danger">{applyIac.error.message}</span>
          {applyCode === 'plan_stale' ? (
            <Button size="sm" type="button" onClick={() => runPlan(yaml)}>
              <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
              {t('iacStudio.replan')}
            </Button>
          ) : null}
        </div>
      ) : null}
      {applyIac.data ? <IacResultView results={applyIac.data.results} /> : null}

      <Button
        type="button"
        disabled={!canApply}
        onClick={() =>
          plan &&
          applyIac.mutate({
            files: [{ name: studioName, content: yaml }],
            expected_plan_hash: plan.hash,
          })
        }
      >
        {applyIac.isPending ? t('iacPage.planning') : t('iacPage.apply')}
      </Button>
    </div>
  )
}

function ChangeCodes({ changes }: Readonly<{ changes: IacChange[] }>) {
  const failed = changes.filter((c) => c.code)
  if (failed.length === 0) return null
  return (
    <ul className="space-y-1 text-xs">
      {failed.map((c) => (
        <li
          key={`${c.kind}/${c.name}`}
          className="flex flex-wrap items-center gap-2"
        >
          <IacCodeBadge code={c.code ?? ''} />
          <span className="font-mono text-muted-foreground">
            {c.kind}/{c.name}
          </span>
        </li>
      ))}
    </ul>
  )
}
