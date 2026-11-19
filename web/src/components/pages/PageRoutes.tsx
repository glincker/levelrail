import { useMemo, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, SignpostGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '@/lib/apiError'
import { toast, toastAction } from '@/components/ui/toast'
import { InfoTip } from '@/components/kit/InfoTip'
import { useAppListOptional } from '../../queries/apps'
import { useTestStaticRoutes } from '../../queries/staticRoutes'
import {
  RulesSaveError,
  useSaveSiteRules,
  useSiteRules,
  type RuleSaveProblem,
  type SiteRules,
} from '../../queries/pagesSites'
import {
  HeaderEditorRow,
  ReadOnlyHeader,
  KindChip,
  ReadOnlyRedirect,
  RedirectEditorRow,
} from './RuleRows'
import {
  draftFromUi,
  draftToUi,
  moveItem,
  newHeader,
  newRedirect,
  sameRules,
  type Draft,
} from './rulesDraft'
import {
  PRESETS,
  buildProxy,
  matchPath,
  targetState,
  validateRow,
  type MatchRule,
  type Preset,
} from './routesLogic'
import { ProblemList } from '../static/ProblemList'
import { StaticRoutesTester } from '../static/StaticRoutesTester'

function TestField({
  app,
  path,
  onPath,
  live,
}: Readonly<{
  app: string
  path: string
  onPath: (p: string) => void
  live: ReturnType<typeof matchPath>
}>) {
  const { t } = useTranslation('pagesSites')
  const test = useTestStaticRoutes(app)
  const invalid = !path.startsWith('/')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (invalid || test.isPending) return
    test.mutate({ path, query: '' }, { onSuccess: setD })
  }
  // Keep the last result on screen while a new test runs so the row never blanks or flashes.
  const [d, setD] = useState<typeof test.data>()
  return (
    <form onSubmit={submit} className="space-y-2">
      <div className="flex gap-2">
        <Input
          aria-label={t('routes.testLabel')}
          value={path}
          placeholder={t('routes.testPlaceholder')}
          aria-invalid={invalid}
          className="h-9 flex-1 font-mono"
          onChange={(e) => onPath(e.target.value)}
        />
        <Button
          type="submit"
          size="sm"
          className="h-9"
          disabled={invalid}
          aria-busy={test.isPending}
        >
          {t('routes.test')}
        </Button>
      </div>
      <p
        className="flex min-h-5 flex-wrap items-center gap-2 text-sm"
        aria-live="polite"
      >
        {live ? (
          <>
            <KindChip kind={live.kind} />
            <Badge variant={live.status >= 400 ? 'warning' : 'success'}>
              {live.status}
            </Badge>
            <span className="font-mono text-xs break-all">{live.target}</span>
          </>
        ) : (
          <span className="text-xs text-muted-foreground">
            {t('routes.noMatch')}
          </span>
        )}
      </p>
      {test.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{test.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {d ? (
        <p className="flex flex-wrap items-center gap-2 text-xs">
          <span className="text-muted-foreground">{t('routes.served')}</span>
          <Badge variant={d.status >= 400 ? 'warning' : 'success'}>
            {d.status}
          </Badge>
          <span className="font-mono break-all">
            {d.kind === 'redirect' ? d.location : (d.file ?? path)}
          </span>
          {d.rule ? (
            <span className="text-muted-foreground">{d.rule}</span>
          ) : null}
        </p>
      ) : null}
    </form>
  )
}

function problemMap(
  problems: readonly RuleSaveProblem[],
  kind: RuleSaveProblem['kind'],
): Map<number, string> {
  const m = new Map<number, string>()
  for (const p of problems) {
    if (p.kind === kind && p.index >= 0) m.set(p.index, p.message)
  }
  return m
}

function initialDraft(d: SiteRules, from?: string): Draft | null {
  if (!from || d.ui.redirects.some((r) => r.from === from)) return null
  const base = draftFromUi(d.ui)
  return { ...base, redirects: [...base.redirects, newRedirect(from)] }
}

function AddButton({
  label,
  onClick,
}: Readonly<{ label: string; onClick: () => void }>) {
  return (
    <Button type="button" size="sm" variant="outline" onClick={onClick}>
      <PlusGlassIcon className="size-4" aria-hidden="true" />
      {label}
    </Button>
  )
}

function RulesEditor({
  app,
  data,
  apps,
  redirectFrom,
}: Readonly<{
  app: string
  data: SiteRules
  apps: readonly string[]
  redirectFrom?: string
}>) {
  const { t } = useTranslation('pagesSites')
  const save = useSaveSiteRules(app)
  const [draft, setDraft] = useState<Draft | null>(() =>
    initialDraft(data, redirectFrom),
  )
  const [problems, setProblems] = useState<RuleSaveProblem[]>([])
  const [attempted, setAttempted] = useState(false)
  const [testPath, setTestPath] = useState('/')
  const [dragFrom, setDragFrom] = useState<number | null>(null)
  const [dragOver, setDragOver] = useState<number | null>(null)
  const saved = useMemo(() => draftFromUi(data.ui), [data.ui])
  const cur = draft ?? saved
  const dirty = draft !== null && !sameRules(draftToUi(draft), draftToUi(saved))
  const edit = (next: Draft) => {
    setDraft(next)
    setProblems([])
  }
  const rErr = problemMap(problems, 'redirect')
  const hErr = problemMap(problems, 'header')
  const general = problems.filter((p) => p.index < 0)
  const lockedRedirects = data.redirects.filter((r) => r.source !== 'ui')
  const lockedHeaders = data.headers.filter((h) => h.source !== 'ui')
  const clientErr = (r: Draft['redirects'][number]): string | undefined => {
    if (!attempted && r.from === '' && r.to === '') return undefined
    const p = validateRow(r, app, apps)
    return p ? t(`routes.problem.${p}`) : undefined
  }
  const blocked = cur.redirects.some((r) => validateRow(r, app, apps) !== null)
  const rules: MatchRule[] = [
    ...cur.redirects.map((r) => ({
      from: r.from.trim(),
      to: r.to.trim(),
      status: Number(r.status),
    })),
    ...lockedRedirects,
  ]
  const live = testPath.startsWith('/') ? matchPath(rules, testPath) : null
  const addRedirect = () =>
    edit({ ...cur, redirects: [...cur.redirects, newRedirect()] })
  const addHeader = () =>
    edit({ ...cur, headers: [...cur.headers, newHeader()] })
  const addPreset = (preset: Preset) => {
    const row = {
      ...newRedirect(preset.from),
      to: preset.id === 'api' ? buildProxy('', '/:splat') : preset.to,
      status: preset.status,
    }
    edit({ ...cur, redirects: [...cur.redirects, row] })
  }
  const removeRedirect = (i: number) => {
    const gone = cur.redirects[i]
    if (!gone) return
    edit({ ...cur, redirects: cur.redirects.filter((x) => x.id !== gone.id) })
    toast.add({
      title: t('routes.removed'),
      type: 'info',
      actionProps: toastAction(t('routes.undo'), () =>
        setDraft((d) => {
          const base = d ?? saved
          const at = Math.min(i, base.redirects.length)
          const redirects = [...base.redirects]
          redirects.splice(at, 0, gone)
          return { ...base, redirects }
        }),
      ),
    })
  }
  const drop = (to: number) => {
    if (dragFrom !== null && dragFrom !== to) {
      edit({ ...cur, redirects: moveItem(cur.redirects, dragFrom, to) })
    }
    setDragFrom(null)
    setDragOver(null)
  }
  const onSave = () => {
    setAttempted(true)
    if (blocked) {
      toast.add({ title: t('routes.invalid'), type: 'error' })
      return
    }
    save.mutate(draftToUi(cur), {
      onSuccess: () => {
        setDraft(null)
        setProblems([])
        setAttempted(false)
        toast.add({ title: t('routes.saved'), type: 'success' })
      },
      onError: (e) => {
        if (e instanceof RulesSaveError) {
          setProblems(e.problems)
          toast.add({ title: t('routes.invalid'), type: 'error' })
        } else {
          toast.add({ title: e.message, type: 'error' })
        }
      },
    })
  }
  const empty = cur.redirects.length + lockedRedirects.length === 0
  return (
    <>
      {data.problems.length > 0 ? (
        <Alert variant="destructive">
          <AlertDescription>
            <ProblemList problems={data.problems} />
          </AlertDescription>
        </Alert>
      ) : null}
      {general.length > 0 ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>
            {general.map((p) => (
              <p key={p.message}>{p.message}</p>
            ))}
          </AlertDescription>
        </Alert>
      ) : null}
      <section className="space-y-2 rounded-lg border border-border bg-card p-3">
        <h2 className="flex items-center gap-1.5 text-sm font-semibold">
          {t('routes.testTitle')}
          <InfoTip>{t('routes.testHint')}</InfoTip>
        </h2>
        <TestField app={app} path={testPath} onPath={setTestPath} live={live} />
      </section>
      <section className="space-y-2 rounded-lg border border-border bg-card p-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="flex items-center gap-1.5 text-sm font-semibold">
            {t('routes.rules')}
            <InfoTip>{t('routes.rulesHint')}</InfoTip>
          </h2>
          <AddButton label={t('routes.addRedirect')} onClick={addRedirect} />
        </div>
        {empty ? (
          <EmptyState
            compact
            icon={<SignpostGlassIcon className="size-6" />}
            title={t('routes.none')}
            action={
              <div className="flex flex-wrap justify-center gap-2">
                {PRESETS.map((p) => (
                  <Button
                    key={p.id}
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => addPreset(p)}
                  >
                    {t(`routes.preset.${p.id}`)}
                  </Button>
                ))}
                <Button size="sm" onClick={addRedirect}>
                  {t('routes.addRedirect')}
                </Button>
              </div>
            }
          />
        ) : (
          <ul className="divide-y divide-border rounded-md border border-border">
            {cur.redirects.map((r, i) => (
              <RedirectEditorRow
                key={r.id}
                row={r}
                index={i}
                count={cur.redirects.length}
                error={rErr.get(i) ?? clientErr(r)}
                apps={apps}
                self={app}
                highlight={live?.index === i}
                dnd={{
                  dragging: dragFrom === i,
                  over: dragFrom !== null && dragFrom !== i && dragOver === i,
                  onStart: () => setDragFrom(i),
                  onOver: () => setDragOver(i),
                  onDrop: () => drop(i),
                  onEnd: () => {
                    setDragFrom(null)
                    setDragOver(null)
                  },
                }}
                onChange={(patch) =>
                  edit({
                    ...cur,
                    redirects: cur.redirects.map((x) =>
                      x.id === r.id ? { ...x, ...patch } : x,
                    ),
                  })
                }
                onMove={(to) =>
                  edit({ ...cur, redirects: moveItem(cur.redirects, i, to) })
                }
                onRemove={() => removeRedirect(i)}
                onEnterLast={addRedirect}
              />
            ))}
            {lockedRedirects.map((r, i) => (
              <ReadOnlyRedirect
                key={`${r.source}:${r.from}:${r.to}`}
                rule={r}
                highlight={live?.index === cur.redirects.length + i}
                state={r.target_state ?? targetState(r.to, app, apps)}
              />
            ))}
          </ul>
        )}
      </section>
      <section className="space-y-2 rounded-lg border border-border bg-card p-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-sm font-semibold">{t('routes.headers')}</h2>
          <AddButton label={t('routes.addHeader')} onClick={addHeader} />
        </div>
        {cur.headers.length + lockedHeaders.length === 0 ? (
          <EmptyState
            compact
            icon={<SignpostGlassIcon className="size-6" />}
            title={t('routes.noHeaders')}
            action={
              <Button size="sm" onClick={addHeader}>
                {t('routes.addHeader')}
              </Button>
            }
          />
        ) : (
          <ul className="divide-y divide-border rounded-md border border-border">
            {cur.headers.map((h, i) => (
              <HeaderEditorRow
                key={h.id}
                row={h}
                index={i}
                count={cur.headers.length}
                error={hErr.get(i)}
                onChange={(patch) =>
                  edit({
                    ...cur,
                    headers: cur.headers.map((x) =>
                      x.id === h.id ? { ...x, ...patch } : x,
                    ),
                  })
                }
                onMove={(to) =>
                  edit({ ...cur, headers: moveItem(cur.headers, i, to) })
                }
                onRemove={() =>
                  edit({
                    ...cur,
                    headers: cur.headers.filter((x) => x.id !== h.id),
                  })
                }
              />
            ))}
            {lockedHeaders.map((h) => (
              <ReadOnlyHeader key={`${h.source}:${h.path}`} rule={h} />
            ))}
          </ul>
        )}
      </section>
      {dirty || save.isPending ? (
        <div
          role="region"
          aria-label={t('routes.unsaved')}
          className="sticky bottom-3 z-10 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-popover px-3 py-2 shadow-md"
        >
          <p className="text-sm" aria-live="polite">
            {save.isPending ? t('routes.saving') : t('routes.unsaved')}
          </p>
          <div className="flex gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={save.isPending}
              onClick={() => {
                setDraft(null)
                setProblems([])
              }}
            >
              {t('routes.discard')}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={save.isPending}
              onClick={onSave}
            >
              {t('routes.save')}
            </Button>
          </div>
        </div>
      ) : null}
    </>
  )
}

/** Routes tab: URL tester first, then the editable UI rules next to the read-only file and app.yaml ones. */
export function PageRoutes({
  app,
  redirectFrom,
}: Readonly<{ app: string; redirectFrom?: string }>) {
  const { t } = useTranslation('pagesSites')
  const q = useSiteRules(app)
  const list = useAppListOptional()
  const apps = useMemo(() => (list.data ?? []).map((a) => a.name), [list.data])
  if (q.isPending) return <Skeleton className="h-40 w-full" />
  if (q.isError) {
    const notStatic = q.error instanceof ApiError && q.error.status === 404
    return (
      <EmptyState
        compact
        icon={<SignpostGlassIcon className="size-6" />}
        title={notStatic ? t('routes.notStatic') : t('routes.loadFailed')}
        action={
          notStatic ? undefined : (
            <Button
              size="sm"
              variant="outline"
              onClick={() => void q.refetch()}
            >
              {t('routes.retry')}
            </Button>
          )
        }
      />
    )
  }
  return (
    <div className="space-y-3">
      <RulesEditor
        key={redirectFrom ?? ''}
        app={app}
        data={q.data}
        apps={apps}
        redirectFrom={redirectFrom}
      />
      <Collapsible>
        <CollapsibleTrigger className="px-1 text-xs text-muted-foreground">
          {t('routes.advanced')}
        </CollapsibleTrigger>
        <CollapsiblePanel className="pt-2">
          <StaticRoutesTester app={app} />
        </CollapsiblePanel>
      </Collapsible>
    </div>
  )
}

export default PageRoutes
