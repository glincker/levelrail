import { useTranslation } from 'react-i18next'
import { SignpostIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '@/lib/apiError'
import {
  useStaticRoutes,
  type StaticHeaderBlock,
  type StaticOptions,
  type StaticRule,
} from '../../queries/staticRoutes'
import { ProblemList } from './ProblemList'
import { StaticRoutesTester } from './StaticRoutesTester'

function kindKey(status: number): 'kindRewrite' | 'kindRedirect' | 'kindPage' {
  if (status === 200) return 'kindRewrite'
  return status >= 300 && status < 400 ? 'kindRedirect' : 'kindPage'
}

function OptionRow({
  label,
  value,
}: Readonly<{ label: string; value: string }>) {
  return (
    <div className="flex items-baseline justify-between gap-3 border-b border-border py-1.5 last:border-0">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium">{value}</dd>
    </div>
  )
}

function OptionsCard({ options }: Readonly<{ options: StaticOptions }>) {
  const { t } = useTranslation('staticRoutes')
  const onOff = (v?: boolean) => (v ? t('options.on') : t('options.off'))
  const slash =
    options.trailing_slash === 'add'
      ? t('options.slashAdd')
      : options.trailing_slash === 'remove'
        ? t('options.slashRemove')
        : t('options.slashOff')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('options.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <dl>
          <OptionRow label={t('options.spa')} value={onOff(options.spa)} />
          <OptionRow
            label={t('options.cleanUrls')}
            value={onOff(options.clean_urls)}
          />
          <OptionRow label={t('options.trailingSlash')} value={slash} />
          <OptionRow
            label={t('options.notFound')}
            value={options.not_found || t('options.defaultNotFound')}
          />
        </dl>
        <p className="mt-3 text-xs text-muted-foreground">
          {t('precedence.body')}
        </p>
      </CardContent>
    </Card>
  )
}

function RulesTable({ rules }: Readonly<{ rules: StaticRule[] }>) {
  const { t } = useTranslation('staticRoutes')
  const columns: ResponsiveColumn<StaticRule>[] = [
    {
      id: 'from',
      header: t('rules.from'),
      priority: 1,
      primary: true,
      cell: (r) => (
        <code className="font-mono text-xs break-all">{r.from}</code>
      ),
    },
    {
      id: 'to',
      header: t('rules.to'),
      priority: 1,
      cell: (r) => <code className="font-mono text-xs break-all">{r.to}</code>,
    },
    {
      id: 'status',
      header: t('rules.status'),
      priority: 1,
      cell: (r) => (
        <span className="flex flex-wrap items-center gap-1.5">
          <Badge variant={r.status === 200 ? 'info' : 'muted'}>
            {r.status} {t(`rules.${kindKey(r.status)}`)}
          </Badge>
          {r.force ? (
            <Badge variant="warning">{t('rules.forced')}</Badge>
          ) : null}
        </span>
      ),
    },
    {
      id: 'source',
      header: t('rules.source'),
      priority: 2,
      cell: (r) => (
        <span className="font-mono text-xs text-muted-foreground">
          {r.source}
        </span>
      ),
    },
  ]
  return (
    <ResponsiveTable
      rows={rules}
      columns={columns}
      getRowId={(r) => r.source}
      ariaLabel={t('rules.label')}
    />
  )
}

function HeadersTable({ blocks }: Readonly<{ blocks: StaticHeaderBlock[] }>) {
  const { t } = useTranslation('staticRoutes')
  const columns: ResponsiveColumn<StaticHeaderBlock>[] = [
    {
      id: 'path',
      header: t('headers.path'),
      priority: 1,
      primary: true,
      cell: (b) => (
        <code className="font-mono text-xs break-all">{b.path}</code>
      ),
    },
    {
      id: 'set',
      header: t('headers.set'),
      priority: 1,
      cell: (b) => (
        <span className="grid gap-0.5">
          {(b.set ?? []).map((h) => (
            <code key={h.name} className="font-mono text-xs break-all">
              {h.name}: {h.value}
            </code>
          ))}
        </span>
      ),
    },
    {
      id: 'remove',
      header: t('headers.remove'),
      priority: 2,
      cell: (b) => (b.remove ?? []).join(', '),
    },
    {
      id: 'source',
      header: t('headers.source'),
      priority: 3,
      cell: (b) => (
        <span className="font-mono text-xs text-muted-foreground">
          {b.source}
        </span>
      ),
    },
  ]
  return (
    <ResponsiveTable
      rows={blocks}
      columns={columns}
      getRowId={(b) => b.source}
      ariaLabel={t('headers.label')}
    />
  )
}

export function StaticRoutesPanel({ app }: Readonly<{ app: string }>) {
  const { t } = useTranslation('staticRoutes')
  const q = useStaticRoutes(app)

  if (q.isPending) return <Skeleton className="h-64 w-full" />
  if (q.isError) {
    const notStatic = q.error instanceof ApiError && q.error.status === 404
    return notStatic ? (
      <EmptyState
        icon={<SignpostIcon className="size-5" />}
        title={t('notStatic.title')}
        description={t('notStatic.description')}
      />
    ) : (
      <Alert variant="destructive">
        <AlertTitle>{t('loadFailed')}</AlertTitle>
        <AlertDescription>
          <Button size="sm" variant="outline" onClick={() => void q.refetch()}>
            {t('retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  const d = q.data
  return (
    <div className="grid gap-4">
      <div>
        <h1 className="text-lg font-semibold">{t('title')}</h1>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </div>
      {d.published ? null : (
        <Alert>
          <AlertTitle>{t('notPublished.title')}</AlertTitle>
          <AlertDescription>{t('notPublished.description')}</AlertDescription>
        </Alert>
      )}
      {d.problems.length > 0 ? (
        <Alert variant="destructive">
          <AlertTitle>{t('problems.title')}</AlertTitle>
          <AlertDescription>
            <p className="mb-2">{t('problems.description')}</p>
            <ProblemList problems={d.problems} />
          </AlertDescription>
        </Alert>
      ) : null}
      <OptionsCard options={d.options} />
      <Card>
        <CardHeader>
          <CardTitle>{t('rules.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          {d.rules.length === 0 ? (
            <EmptyState
              compact
              icon={<SignpostIcon className="size-5" />}
              title={t('rules.empty.title')}
              description={t('rules.empty.description')}
            />
          ) : (
            <RulesTable rules={d.rules} />
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('headers.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          {d.headers.length === 0 ? (
            <EmptyState
              compact
              icon={<SignpostIcon className="size-5" />}
              title={t('headers.empty.title')}
              description={t('headers.empty.description')}
            />
          ) : (
            <HeadersTable blocks={d.headers} />
          )}
        </CardContent>
      </Card>
      <StaticRoutesTester app={app} />
    </div>
  )
}
