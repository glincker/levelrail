import { useMemo, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { ArrowLeftRightGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  isSelectableItem,
  useImportMigration,
  usePlanMigration,
  type MigrateItem,
  type MigrateOverride,
  type MigratePlatform,
  type MigrateReport,
  type MigrateRequest,
  type MigrateVerdict,
} from '../../queries/migrateWizard'
import { MigrateItemRow } from './MigrateItemRow'

const PLATFORMS: { id: MigratePlatform; label: string }[] = [
  { id: 'coolify', label: 'Coolify' },
  { id: 'dokploy', label: 'Dokploy' },
  { id: 'caprover', label: 'CapRover' },
]

const VERDICT_ORDER: MigrateVerdict[] = [
  'will_import',
  'imports_with_changes',
  'needs_manual',
  'unsupported',
  'already_imported',
]

const VIRTUALIZE_ABOVE = 50

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : ''
}

function VirtualRows({
  items,
  render,
}: {
  items: MigrateItem[]
  render: (item: MigrateItem) => React.ReactNode
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 140,
    overscan: 6,
  })
  return (
    <div ref={parentRef} className="max-h-[70vh] overflow-auto">
      <div className="relative" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((row) => {
          const item = items[row.index]
          if (!item) return null
          return (
            <div
              key={row.key}
              ref={virtualizer.measureElement}
              data-index={row.index}
              className="absolute top-0 left-0 w-full"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              {render(item)}
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function MigratePlan({ onOpenCutover }: { onOpenCutover: () => void }) {
  const { t } = useTranslation('migrate')
  const [platform, setPlatform] = useState<MigratePlatform>('coolify')
  const [url, setUrl] = useState('')
  const [token, setToken] = useState('')
  const [insecure, setInsecure] = useState(false)
  const [overrides, setOverrides] = useState<Record<string, MigrateOverride>>(
    {},
  )
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [report, setReport] = useState<MigrateReport | null>(null)
  const [outcome, setOutcome] = useState<{
    report: MigrateReport
    dryRun: boolean
  } | null>(null)
  const plan = usePlanMigration()
  const run = useImportMigration()
  const busy = plan.isPending || run.isPending

  function base(over: Record<string, MigrateOverride>): MigrateRequest {
    return {
      platform,
      url: url.trim(),
      token,
      insecure_tls: insecure || undefined,
      overrides: over,
    }
  }

  function readSource(e: FormEvent) {
    e.preventDefault()
    setOutcome(null)
    setOverrides({})
    plan.mutate(base({}), {
      onSuccess: (r) => {
        setReport(r)
        setSelected(
          new Set(r.items.filter(isSelectableItem).map((i) => i.source_id)),
        )
      },
    })
  }

  function applyFix(item: MigrateItem, patch: MigrateOverride) {
    const next = {
      ...overrides,
      [item.source_id]: { ...overrides[item.source_id], ...patch },
    }
    plan.mutate(base(next), {
      onSuccess: (r) => {
        setOverrides(next)
        setReport(r)
        setOutcome(null)
        toast.add({ title: t('fix.applied'), type: 'success' })
      },
      onError: (e) => toast.add({ title: errorMessage(e), type: 'error' }),
    })
  }

  function toggle(id: string) {
    setSelected((cur) => {
      const next = new Set(cur)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function submit(dryRun: boolean) {
    if (selected.size === 0) {
      toast.add({ title: t('import.noneSelected'), type: 'error' })
      return
    }
    run.mutate(
      {
        ...base(overrides),
        only: [...selected],
        dry_run: dryRun,
      },
      {
        onSuccess: (r) => {
          setOutcome({ report: r, dryRun })
          if (!dryRun) {
            setToken('')
            setReport(r)
            setSelected(new Set())
          }
        },
        onError: (e) => toast.add({ title: errorMessage(e), type: 'error' }),
      },
    )
  }

  const items = useMemo(() => report?.items ?? [], [report])
  const rows = (item: MigrateItem) => (
    <MigrateItemRow
      key={item.source_id}
      item={item}
      selected={selected.has(item.source_id)}
      busy={busy}
      onToggle={() => toggle(item.source_id)}
      onFix={(patch) => applyFix(item, patch)}
      onOpenCutover={onOpenCutover}
    />
  )

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <CardTitle>{t('connect.title')}</CardTitle>
          <p className="text-sm text-muted-foreground">
            {t('connect.description')}
          </p>
        </CardHeader>
        <CardContent>
          <form className="space-y-3" onSubmit={readSource}>
            <div
              className="flex flex-wrap gap-2"
              role="group"
              aria-label={t('connect.platform')}
            >
              {PLATFORMS.map((p) => (
                <Button
                  key={p.id}
                  type="button"
                  size="sm"
                  variant={platform === p.id ? 'default' : 'outline'}
                  aria-pressed={platform === p.id}
                  onClick={() => setPlatform(p.id)}
                >
                  {p.label}
                </Button>
              ))}
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="migrate-url">{t('connect.url')}</Label>
                <Input
                  id="migrate-url"
                  required
                  placeholder={t('connect.urlPlaceholder')}
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="migrate-token">{t('connect.token')}</Label>
                <Input
                  id="migrate-token"
                  type="password"
                  required
                  autoComplete="off"
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                />
              </div>
            </div>
            <Label className="flex items-center gap-2 text-sm font-normal">
              <Checkbox
                aria-label={t('connect.insecure')}
                checked={insecure}
                onCheckedChange={(v) => setInsecure(v === true)}
              />
              {t('connect.insecure')}
            </Label>
            <Button type="submit" disabled={busy}>
              {plan.isPending ? t('connect.reading') : t('connect.submit')}
            </Button>
            {plan.isError && !report ? (
              <Banner variant="critical" title={t('connect.error')}>
                {errorMessage(plan.error)}
              </Banner>
            ) : null}
          </form>
        </CardContent>
      </Card>

      {report === null ? (
        <EmptyState
          icon={<ArrowLeftRightGlassIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.description')}
        />
      ) : (
        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle>
                {t('summary.items', { count: items.length })}
              </CardTitle>
              {VERDICT_ORDER.filter((v) => (report.counts[v] ?? 0) > 0).map(
                (v) => (
                  <Badge key={v} variant="outline">
                    {t(`verdict.${v}`)}: {report.counts[v]}
                  </Badge>
                ),
              )}
              <span className="ml-auto text-sm text-muted-foreground">
                {t('summary.selected', { count: selected.size })}
              </span>
            </div>
            {(report.notes ?? []).map((n) => (
              <Banner key={n} variant="warning" title={n} />
            ))}
          </CardHeader>
          <CardContent className="p-0">
            {items.length > VIRTUALIZE_ABOVE ? (
              <VirtualRows items={items} render={rows} />
            ) : (
              items.map(rows)
            )}
            <div className="flex flex-wrap gap-2 border-t p-3">
              <Button
                variant="outline"
                disabled={busy}
                onClick={() => submit(true)}
              >
                {t('import.preview')}
              </Button>
              <Button disabled={busy} onClick={() => submit(false)}>
                {run.isPending ? t('import.running') : t('import.run')}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {outcome ? (
        <Banner
          variant={outcome.dryRun ? 'info' : 'success'}
          title={
            outcome.dryRun ? t('import.previewTitle') : t('import.doneTitle')
          }
          action={
            outcome.dryRun ? null : (
              <Button size="sm" variant="outline" onClick={onOpenCutover}>
                {t('import.openCutover')}
              </Button>
            )
          }
        >
          {outcome.dryRun ? (
            <ul className="list-disc pl-5 text-sm">
              {outcome.report.items
                .filter((i) => selected.has(i.source_id))
                .map((i) => (
                  <li key={i.source_id}>
                    {i.name}: {t(`verdict.${i.verdict}`)}
                  </li>
                ))}
            </ul>
          ) : (
            t('import.doneHint')
          )}
        </Banner>
      ) : null}
      {run.isError && !outcome ? (
        <Banner variant="critical" title={t('import.error')}>
          {errorMessage(run.error)}
        </Banner>
      ) : null}
    </div>
  )
}
