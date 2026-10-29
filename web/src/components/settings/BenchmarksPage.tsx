import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChartColumnGlassIcon,
  EyeGlassIcon,
  RefreshCwGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { PageBanner } from '@/components/PageBanner'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useBrand } from '../../hooks/useBrand'
import { cliCommand } from '../../lib/cliCommand'
import { orderedPair, shortCommit } from '../../lib/benchmarkFormat'
import {
  useBenchmarkComparison,
  useBenchmarkRun,
  useBenchmarkRuns,
  useDeleteBenchmark,
  type BenchmarkRunSummary,
} from '../../queries/benchmarks'

const RunDetail = lazy(() =>
  import('./BenchmarkRunDetail').then((m) => ({
    default: m.BenchmarkRunDetail,
  })),
)
const Compare = lazy(() =>
  import('./BenchmarkCompare').then((m) => ({ default: m.BenchmarkCompare })),
)

function DetailPane({ id }: { id: string }) {
  const { t } = useTranslation('resilience', { keyPrefix: 'benchmarks' })
  const { data, isPending, error } = useBenchmarkRun(id)
  if (isPending) return <Skeleton className="h-64 w-full" />
  if (error) {
    return (
      <Banner variant="critical" title={t('loadError')}>
        {error.message}
      </Banner>
    )
  }
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <RunDetail run={data} />
    </Suspense>
  )
}

function ComparePane({ base, head }: { base: string; head: string }) {
  const { t } = useTranslation('resilience', { keyPrefix: 'benchmarks' })
  const { data, isPending, error } = useBenchmarkComparison(base, head)
  if (isPending) return <Skeleton className="h-64 w-full" />
  if (error) {
    return (
      <Banner variant="critical" title={t('loadError')}>
        {error.message}
      </Banner>
    )
  }
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <Compare data={data} />
    </Suspense>
  )
}

export function BenchmarksPage() {
  const { t } = useTranslation('resilience', { keyPrefix: 'benchmarks' })
  const brand = useBrand()
  const { data, isPending, error, refetch, isFetching } = useBenchmarkRuns()
  const del = useDeleteBenchmark()
  const [viewing, setViewing] = useState<string | null>(null)
  const [picked, setPicked] = useState<string[]>([])
  const [comparing, setComparing] = useState<[string, string] | null>(null)
  const [pendingDelete, setPendingDelete] =
    useState<BenchmarkRunSummary | null>(null)

  const runs = data?.runs ?? []
  const nonLinux = runs.some((r) => r.host_os !== 'linux')

  const toggle = (id: string, on: boolean) => {
    setComparing(null)
    setPicked((cur) =>
      on ? [...cur, id].slice(-2) : cur.filter((x) => x !== id),
    )
  }

  const startCompare = () => {
    const a = runs.find((r) => r.id === picked[0])
    const b = runs.find((r) => r.id === picked[1])
    if (!a || !b) return
    const [base, head] = orderedPair(a, b)
    setViewing(null)
    setComparing([base.id, head.id])
  }

  const confirmDelete = () => {
    if (!pendingDelete) return
    const id = pendingDelete.id
    del.mutate(id, {
      onSuccess: () => {
        setPendingDelete(null)
        setPicked((cur) => cur.filter((x) => x !== id))
        if (viewing === id) setViewing(null)
        setComparing(null)
        toast.add({ title: t('deleted'), type: 'success' })
      },
      onError: (e) => {
        toast.add({
          title: t('deleteFailed'),
          description: e.message,
          type: 'error',
        })
      },
    })
  }

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <PageBanner
        icon={<ChartColumnGlassIcon size="1.4em" />}
        titleKey="resilience:benchmarks.title"
        descriptionKey="resilience:benchmarks.description"
        size="compact"
        action={
          <Button
            type="button"
            size="sm"
            disabled={isFetching}
            onClick={() => {
              void refetch()
            }}
          >
            <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
            {t('refresh')}
          </Button>
        }
      />
      <Banner variant="warning" title={t('standingCaveat.title')}>
        {t('standingCaveat.body')}
      </Banner>
      {isPending ? <Skeleton className="h-40 w-full" /> : null}
      {error ? (
        <Banner variant="critical" title={t('loadError')}>
          {error.message}
        </Banner>
      ) : null}
      {data && runs.length === 0 ? (
        <EmptyState
          icon={<ChartColumnGlassIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.body')}
        >
          <code className="text-xs break-all">
            {cliCommand(
              brand.BinaryName,
              'bench chaos run all --repeat 5 --out results.json --upload',
            )}
          </code>
        </EmptyState>
      ) : null}
      {runs.length > 0 ? (
        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <CardTitle>{t('history.title')}</CardTitle>
              <Button
                type="button"
                size="sm"
                disabled={picked.length !== 2}
                onClick={startCompare}
              >
                {t('history.compare')}
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              {t('history.keep', { count: data?.keep ?? 0 })}
              {nonLinux ? ` ${t('history.nonLinux')}` : ''}
            </p>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <span className="sr-only">{t('history.select')}</span>
                  </TableHead>
                  <TableHead>{t('history.when')}</TableHead>
                  <TableHead>{t('history.build')}</TableHead>
                  <TableHead>{t('history.host')}</TableHead>
                  <TableHead>{t('history.result')}</TableHead>
                  <TableHead className="text-right">
                    <span className="sr-only">{t('history.actions')}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((r) => (
                  <TableRow
                    key={r.id}
                    data-state={r.id === viewing ? 'selected' : undefined}
                  >
                    <TableCell>
                      <Checkbox
                        checked={picked.includes(r.id)}
                        onCheckedChange={(on) => toggle(r.id, on)}
                        aria-label={t('history.selectRun', { id: r.id })}
                      />
                    </TableCell>
                    <TableCell className="tabular-nums">
                      {new Date(r.generated_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <span className="font-medium">{r.version}</span>{' '}
                      <span className="font-mono text-xs text-muted-foreground">
                        {shortCommit(r.commit, r.dirty)}
                      </span>
                    </TableCell>
                    <TableCell className="max-w-xs text-muted-foreground">
                      {r.host_summary}
                    </TableCell>
                    <TableCell className="space-x-1">
                      <Badge variant="success">
                        {t('history.passed', { count: r.passed })}
                      </Badge>
                      {r.failed > 0 ? (
                        <Badge variant="destructive">
                          {t('history.failed', { count: r.failed })}
                        </Badge>
                      ) : null}
                      <Badge variant="outline">
                        {t('history.repeat', { count: r.repeat })}
                      </Badge>
                    </TableCell>
                    <TableCell className="space-x-1 text-right whitespace-nowrap">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setComparing(null)
                          setViewing(r.id)
                        }}
                      >
                        <EyeGlassIcon size="1.4em" aria-hidden="true" />
                        {t('history.view')}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        aria-label={t('history.deleteRun', { id: r.id })}
                        onClick={() => setPendingDelete(r)}
                      >
                        <TrashGlassIcon size="1.4em" aria-hidden="true" />
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      ) : null}
      {viewing ? <DetailPane id={viewing} /> : null}
      {comparing ? (
        <ComparePane base={comparing[0]} head={comparing[1]} />
      ) : null}
      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null)
        }}
        title={t('confirmDelete.title')}
        description={t('confirmDelete.body', { id: pendingDelete?.id ?? '' })}
        confirmLabel={t('confirmDelete.confirm')}
        tone="destructive"
        pending={del.isPending}
        onConfirm={confirmDelete}
      />
    </div>
  )
}
