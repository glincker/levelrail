import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  useLintStaticRoutes,
  useTestStaticRoutes,
  type StaticRouteDecision,
} from '../../queries/staticRoutes'
import { ProblemList } from './ProblemList'

function DecisionView({ d }: Readonly<{ d: StaticRouteDecision }>) {
  const { t } = useTranslation('staticRoutes')
  let outcome = t('test.plain')
  if (d.kind === 'redirect') {
    outcome = t('test.redirectTo', { target: d.location })
  } else if (d.file) {
    outcome = t('test.serves', { file: d.file })
  }
  return (
    <div className="grid gap-2" aria-live="polite">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant={d.status >= 400 ? 'warning' : 'success'}>
          {d.status}
        </Badge>
        <span className="text-sm font-medium break-all">{outcome}</span>
      </div>
      <p className="text-xs text-muted-foreground">
        {d.rule ? t('test.matchedRule', { rule: d.rule }) : t('test.noRule')}
      </p>
      {d.headers.length > 0 ? (
        <div>
          <p className="text-xs font-medium">{t('test.headersApplied')}</p>
          {d.headers.map((h) => (
            <code key={h.name} className="block font-mono text-xs break-all">
              {h.name}: {h.value}
            </code>
          ))}
        </div>
      ) : null}
      <div>
        <p className="text-xs font-medium">{t('test.trace')}</p>
        <ol className="list-decimal pl-5 text-xs text-muted-foreground">
          {d.trace.map((line) => (
            <li key={line} className="break-all">
              {line}
            </li>
          ))}
        </ol>
      </div>
    </div>
  )
}

function TestBox({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('staticRoutes')
  const test = useTestStaticRoutes(app)
  const [path, setPath] = useState('/')
  const [query, setQuery] = useState('')
  const invalid = !path.startsWith('/')

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!invalid) test.mutate({ path, query })
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('test.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('test.description')}</p>
      </CardHeader>
      <CardContent className="grid gap-3">
        <form
          onSubmit={submit}
          className="grid gap-3 sm:grid-cols-[2fr_1fr_auto] sm:items-end"
        >
          <div className="grid gap-1.5">
            <Label htmlFor="static-routes-test-path">
              {t('test.pathLabel')}
            </Label>
            <Input
              id="static-routes-test-path"
              value={path}
              placeholder={t('test.pathPlaceholder')}
              aria-invalid={invalid}
              onChange={(e) => setPath(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="static-routes-test-query">
              {t('test.queryLabel')}
            </Label>
            <Input
              id="static-routes-test-query"
              value={query}
              placeholder={t('test.queryPlaceholder')}
              onChange={(e) => setQuery(e.target.value)}
            />
          </div>
          <Button type="submit" disabled={invalid || test.isPending}>
            {test.isPending ? t('test.running') : t('test.run')}
          </Button>
        </form>
        {invalid ? (
          <p className="text-xs text-destructive">{t('test.invalidPath')}</p>
        ) : null}
        {test.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {test.error.message || t('test.failed')}
            </AlertDescription>
          </Alert>
        ) : null}
        {test.data ? <DecisionView d={test.data} /> : null}
      </CardContent>
    </Card>
  )
}

function LintBox({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('staticRoutes')
  const lint = useLintStaticRoutes(app)
  const [redirects, setRedirects] = useState('')
  const [headers, setHeaders] = useState('')
  const res = lint.data

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('lint.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('lint.description')}</p>
      </CardHeader>
      <CardContent className="grid gap-3">
        <div className="grid gap-3 md:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="static-routes-lint-redirects">
              {t('lint.redirectsLabel')}
            </Label>
            <Textarea
              id="static-routes-lint-redirects"
              className="font-mono text-xs"
              rows={6}
              value={redirects}
              onChange={(e) => setRedirects(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="static-routes-lint-headers">
              {t('lint.headersLabel')}
            </Label>
            <Textarea
              id="static-routes-lint-headers"
              className="font-mono text-xs"
              rows={6}
              value={headers}
              onChange={(e) => setHeaders(e.target.value)}
            />
          </div>
        </div>
        <div>
          <Button
            variant="outline"
            disabled={lint.isPending || (!redirects && !headers)}
            onClick={() => lint.mutate({ redirects, headers })}
          >
            {lint.isPending ? t('lint.running') : t('lint.run')}
          </Button>
        </div>
        {lint.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {lint.error.message || t('lint.failed')}
            </AlertDescription>
          </Alert>
        ) : null}
        {res?.error ? (
          <Alert variant="destructive">
            <AlertDescription>{res.error}</AlertDescription>
          </Alert>
        ) : null}
        {res && !res.error ? (
          <div className="grid gap-2" aria-live="polite">
            <p className="text-sm">
              {t(res.problems.length > 0 ? 'lint.withProblems' : 'lint.ok', {
                rules: res.rules.length,
                headers: res.headers.length,
                problems: res.problems.length,
              })}
            </p>
            {res.problems.length > 0 ? (
              <ProblemList problems={res.problems} />
            ) : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

export function StaticRoutesTester({ app }: Readonly<{ app: string }>) {
  return (
    <>
      <TestBox app={app} />
      <LintBox app={app} />
    </>
  )
}
