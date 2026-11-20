import { type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { KindChip } from './RuleRows'
import {
  MAX_REQUEST_PATH,
  normalizeTestInput,
  type MatchResult,
} from './routesLogic'

interface Props {
  path: string
  query: string
  onPath: (p: string) => void
  onQuery: (q: string) => void
  /** Result of matchPath for the normalized input, null when nothing matched. */
  live: MatchResult | null
  /** Optional trailing control, such as the server-side test button. */
  action?: ReactNode
}

function Result({ live }: Readonly<{ live: MatchResult }>) {
  const { t } = useTranslation('pagesSites')
  const splat = live.captures.find((c) => c.name === 'splat')
  const named = live.captures.filter((c) => c.name !== 'splat')
  return (
    <div className="space-y-1">
      <p className="flex flex-wrap items-center gap-2 text-sm">
        <KindChip kind={live.kind} />
        <Badge variant={live.status >= 400 ? 'warning' : 'success'}>
          {live.status}
        </Badge>
        <span className="font-mono text-xs break-all">{live.target}</span>
      </p>
      <p className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
        <span>{t('routes.tester.rule', { n: live.index + 1 })}</span>
        <code className="font-mono break-all">{live.from}</code>
        {live.force ? (
          <Badge variant="outline">{t('routes.tester.forced')}</Badge>
        ) : null}
        {splat ? (
          <span>
            {t('routes.tester.splat')}
            <code className="ml-1 font-mono break-all">
              {splat.value || t('routes.tester.empty')}
            </code>
          </span>
        ) : null}
        {named.map((c) => (
          <span key={c.name}>
            <code className="font-mono">:{c.name}</code>
            <code className="ml-1 font-mono break-all">{c.value}</code>
          </span>
        ))}
        {!splat && named.length === 0 ? (
          <span>{t('routes.tester.noCapture')}</span>
        ) : null}
      </p>
      {live.yieldsToFile ? (
        <p className="text-xs text-muted-foreground">
          {t('routes.tester.yields')}
        </p>
      ) : null}
      {live.kind === 'rewrite' || live.kind === 'status' ? (
        <p className="text-xs text-muted-foreground">
          {t(`routes.tester.note.${live.kind}`)}
        </p>
      ) : null}
    </div>
  )
}

export function RedirectTester({
  path,
  query,
  onPath,
  onQuery,
  live,
  action,
}: Readonly<Props>) {
  const { t } = useTranslation('pagesSites')
  const norm = normalizeTestInput(path, query)
  const tooLong = norm.path.length > MAX_REQUEST_PATH
  let body: ReactNode
  if (norm.path === '') {
    body = (
      <span className="text-xs text-muted-foreground">
        {t('routes.tester.empty_input')}
      </span>
    )
  } else if (tooLong) {
    body = (
      <span className="text-xs text-destructive">
        {t('routes.tester.tooLong', { max: MAX_REQUEST_PATH })}
      </span>
    )
  } else if (live) {
    body = <Result live={live} />
  } else {
    body = (
      <span className="text-xs text-muted-foreground">
        {t('routes.tester.none')}
      </span>
    )
  }
  return (
    <div className="space-y-2">
      <div className="grid gap-2 sm:grid-cols-[2fr_1fr_auto] sm:items-end">
        <div className="grid gap-1">
          <Label htmlFor="redirect-tester-path" className="text-xs">
            {t('routes.testLabel')}
          </Label>
          <Input
            id="redirect-tester-path"
            value={path}
            placeholder={t('routes.testPlaceholder')}
            maxLength={MAX_REQUEST_PATH + 512}
            autoComplete="off"
            spellCheck={false}
            className="h-9 font-mono"
            onChange={(e) => onPath(e.target.value)}
          />
        </div>
        <div className="grid gap-1">
          <Label htmlFor="redirect-tester-query" className="text-xs">
            {t('routes.tester.queryLabel')}
          </Label>
          <Input
            id="redirect-tester-query"
            value={query}
            placeholder={t('routes.tester.queryPlaceholder')}
            autoComplete="off"
            spellCheck={false}
            className="h-9 font-mono"
            onChange={(e) => onQuery(e.target.value)}
          />
        </div>
        {action}
      </div>
      <div className="min-h-5" aria-live="polite" role="status">
        {body}
      </div>
      {norm.query && live && live.kind !== 'redirect' ? (
        <p className="text-xs text-muted-foreground">
          {t('routes.tester.queryIgnored', { query: norm.query })}
        </p>
      ) : null}
    </div>
  )
}
