import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CopyGlassIcon,
  SignpostGlassIcon,
} from '@thesvg/ui-react'
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
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import { ApiError } from '@/lib/apiError'
import {
  useStaticRoutes,
  useTestStaticRoutes,
  type StaticRule,
} from '../../queries/staticRoutes'
import { redirectLine } from './pageHelpers'
import { ProblemList } from '../static/ProblemList'
import { StaticRoutesTester } from '../static/StaticRoutesTester'

function TestField({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('pagesSites')
  const test = useTestStaticRoutes(app)
  const [path, setPath] = useState('/')
  const invalid = !path.startsWith('/')
  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!invalid) test.mutate({ path, query: '' })
  }
  const d = test.data
  return (
    <form onSubmit={submit} className="space-y-2">
      <div className="flex gap-2">
        <Input
          aria-label={t('routes.testLabel')}
          value={path}
          placeholder={t('routes.testPlaceholder')}
          aria-invalid={invalid}
          className="h-10 flex-1 font-mono"
          onChange={(e) => setPath(e.target.value)}
        />
        <Button type="submit" disabled={invalid || test.isPending}>
          {t('routes.test')}
        </Button>
      </div>
      {test.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{test.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {d ? (
        <p
          className="flex flex-wrap items-center gap-2 text-sm"
          aria-live="polite"
        >
          <Badge variant={d.status >= 400 ? 'warning' : 'success'}>
            {d.status}
          </Badge>
          <span className="font-mono text-xs break-all">
            {d.kind === 'redirect' ? d.location : (d.file ?? path)}
          </span>
          {d.rule ? (
            <span className="text-xs text-muted-foreground">{d.rule}</span>
          ) : null}
        </p>
      ) : null}
    </form>
  )
}

function AddRule({ initialFrom }: Readonly<{ initialFrom?: string }>) {
  const { t } = useTranslation('pagesSites')
  const { copied, copy } = useCopyToClipboard()
  const [from, setFrom] = useState(initialFrom ?? '')
  const [to, setTo] = useState('/')
  const [status, setStatus] = useState('301')
  const ready = from.startsWith('/') && to.trim() !== ''
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Input
        aria-label={t('routes.from')}
        value={from}
        placeholder="/old"
        className="min-w-32 flex-1 font-mono"
        onChange={(e) => setFrom(e.target.value)}
      />
      <Input
        aria-label={t('routes.to')}
        value={to}
        placeholder="/new"
        className="min-w-32 flex-1 font-mono"
        onChange={(e) => setTo(e.target.value)}
      />
      <select
        aria-label={t('routes.status')}
        value={status}
        onChange={(e) => setStatus(e.target.value)}
        className="h-9 rounded-md border border-input bg-background px-2 text-sm"
      >
        <option value="301">301</option>
        <option value="302">302</option>
        <option value="200">200</option>
      </select>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={!ready}
        onClick={() => copy(redirectLine(from, to, status))}
      >
        {copied ? (
          <CircleCheckGlassIcon className="size-4" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-4" aria-hidden="true" />
        )}
        {t('routes.copyLine')}
      </Button>
    </div>
  )
}

function RuleRow({ rule }: Readonly<{ rule: StaticRule }>) {
  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-1.5 text-xs">
      <code className="font-mono break-all">{rule.from}</code>
      <span aria-hidden="true" className="text-muted-foreground">
        {'→'}
      </span>
      <code className="min-w-0 flex-1 font-mono break-all">{rule.to}</code>
      <Badge variant={rule.status === 200 ? 'info' : 'muted'}>
        {rule.status}
      </Badge>
      {rule.force ? <Badge variant="warning">!</Badge> : null}
    </li>
  )
}

/** Routes tab: a single test field first, rules as compact rows, and a copy-ready add-rule row. */
export function PageRoutes({
  app,
  redirectFrom,
}: Readonly<{ app: string; redirectFrom?: string }>) {
  const { t } = useTranslation('pagesSites')
  const q = useStaticRoutes(app)
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
  const d = q.data
  return (
    <div className="space-y-3">
      <section className="space-y-2 rounded-lg border border-border bg-card p-3">
        <h2 className="text-sm font-semibold">{t('routes.testTitle')}</h2>
        <TestField app={app} />
      </section>
      {d.problems.length > 0 ? (
        <Alert variant="destructive">
          <AlertDescription>
            <ProblemList problems={d.problems} />
          </AlertDescription>
        </Alert>
      ) : null}
      <section className="space-y-2 rounded-lg border border-border bg-card p-3">
        <h2 className="text-sm font-semibold">{t('routes.rules')}</h2>
        {d.rules.length === 0 ? (
          <EmptyState
            compact
            icon={<SignpostGlassIcon className="size-6" />}
            title={t('routes.none')}
          />
        ) : (
          <ul className="divide-y divide-border rounded-md border border-border">
            {d.rules.map((r) => (
              <RuleRow key={r.source} rule={r} />
            ))}
          </ul>
        )}
        <AddRule key={redirectFrom ?? ''} initialFrom={redirectFrom} />
      </section>
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
