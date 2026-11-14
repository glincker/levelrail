import {
  lazy,
  memo,
  Suspense,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import type { KeyboardEvent, Ref } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ClipboardCopyGlassIcon,
  RefreshCwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
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
const VIEW_KEY = 'levelrail.iacStudio.view'
const IacStudioForm = lazy(() => import('./IacStudioForm'))

type StudioView = 'yaml' | 'form'

export interface StudioSeed {
  id: number
  text: string
  exported?: boolean
}

function readView(): StudioView {
  try {
    return localStorage.getItem(VIEW_KEY) === 'form' ? 'form' : 'yaml'
  } catch {
    return 'yaml'
  }
}

function writeView(v: StudioView) {
  try {
    localStorage.setItem(VIEW_KEY, v)
  } catch {
    // Storage can be blocked; the choice just does not persist.
  }
}

// Memoised so a keystroke that keeps the line count does not rebuild the gutter text.
const LineGutter = memo(function LineGutter({
  count,
  gutterRef,
}: Readonly<{ count: number; gutterRef: Ref<HTMLPreElement> }>) {
  return (
    <pre
      ref={gutterRef}
      aria-hidden="true"
      className="max-h-96 min-h-48 overflow-hidden rounded-md border bg-muted/40 px-2 py-2 text-right font-mono text-xs leading-[1.625] text-muted-foreground select-none"
    >
      {Array.from({ length: count }, (_, i) => i + 1).join('\n')}
    </pre>
  )
})

const VIEWS: readonly StudioView[] = ['yaml', 'form']

export function IacStudio({ seed }: Readonly<{ seed?: StudioSeed }>) {
  const { t } = useTranslation('settings')
  const [yaml, setYaml] = useState(seed?.text ?? '')
  const [seedId, setSeedId] = useState(seed?.id)
  const [view, setView] = useState<StudioView>(readView)
  const [copied, setCopied] = useState(false)
  const gutterRef = useRef<HTMLPreElement>(null)
  const viewRefs = useRef<Record<StudioView, HTMLButtonElement | null>>({
    yaml: null,
    form: null,
  })
  if (seed && seed.id !== seedId) {
    setSeedId(seed.id)
    setYaml(seed.text)
    setView('yaml')
  }
  const pickView = (v: StudioView) => {
    setView(v)
    writeView(v)
  }
  // Radiogroup keys: arrows move the choice and focus, Home and End jump.
  function onViewKey(e: KeyboardEvent<HTMLDivElement>) {
    const step = ['ArrowRight', 'ArrowDown'].includes(e.key)
      ? 1
      : ['ArrowLeft', 'ArrowUp'].includes(e.key)
        ? -1
        : 0
    let next: StudioView | null = null
    if (step !== 0) {
      next =
        VIEWS[(VIEWS.indexOf(view) + step + VIEWS.length) % VIEWS.length] ??
        null
    } else if (e.key === 'Home') next = 'yaml'
    else if (e.key === 'End') next = 'form'
    if (!next) return
    e.preventDefault()
    pickView(next)
    viewRefs.current[next]?.focus()
  }
  async function copyYaml() {
    try {
      await navigator.clipboard.writeText(yaml)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      setCopied(false)
    }
  }
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
  const lineCount = useMemo(() => Math.max(1, yaml.split('\n').length), [yaml])

  return (
    <div className="space-y-4">
      <IacTemplateStrip onUse={setYaml} />
      {seed?.exported && seed.id === seedId ? (
        <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/30 p-2 text-xs text-muted-foreground">
          <span>{t('iacStudio.exportedNote')}</span>
          <span role="status" className="sr-only">
            {copied ? t('iacStudio.copied') : ''}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void copyYaml()}
          >
            <ClipboardCopyGlassIcon size="1.4em" aria-hidden="true" />
            {copied ? t('iacStudio.copied') : t('iacStudio.copyYaml')}
          </Button>
        </div>
      ) : null}
      <div
        role="radiogroup"
        aria-label={t('iacStudio.viewLabel')}
        onKeyDown={onViewKey}
        className="inline-flex rounded-md border p-0.5"
      >
        {VIEWS.map((v) => (
          <Button
            key={v}
            ref={(el) => {
              viewRefs.current[v] = el
            }}
            type="button"
            role="radio"
            size="sm"
            variant={view === v ? 'secondary' : 'ghost'}
            aria-checked={view === v}
            tabIndex={view === v ? 0 : -1}
            onClick={() => pickView(v)}
          >
            {t(v === 'yaml' ? 'iacStudio.viewYaml' : 'iacStudio.viewForm')}
          </Button>
        ))}
      </div>
      {view === 'form' ? (
        <Suspense
          fallback={
            <p role="status" className="text-sm text-muted-foreground">
              {t('iacStudio.formLoading')}
            </p>
          }
        >
          <IacStudioForm
            text={yaml}
            onChange={setYaml}
            onEditYaml={() => pickView('yaml')}
          />
        </Suspense>
      ) : (
        <div className="flex gap-2">
          <LineGutter count={lineCount} gutterRef={gutterRef} />
          <Textarea
            aria-label={t('iacStudio.editor')}
            aria-describedby="iac-studio-cloud-note iac-studio-result"
            wrap="off"
            onScroll={(e) => {
              if (gutterRef.current)
                gutterRef.current.scrollTop = e.currentTarget.scrollTop
            }}
            value={yaml}
            onChange={(e) => setYaml(e.target.value)}
            spellCheck={false}
            placeholder={t('iacStudio.placeholder')}
            className="max-h-96 min-h-48 min-w-0 flex-1 overflow-x-auto font-mono text-xs leading-[1.625] whitespace-pre"
          />
        </div>
      )}
      <p id="iac-studio-cloud-note" className="text-xs text-muted-foreground">
        {t('iacStudio.cloudNote')}
      </p>

      <div
        id="iac-studio-result"
        role="region"
        aria-label={t('iacStudio.resultLabel')}
        aria-live="polite"
        className="space-y-3"
      >
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
                <span className="font-mono text-xs break-words text-tone-danger">
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
