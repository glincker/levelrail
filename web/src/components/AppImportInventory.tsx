import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  usePutAppImportPlan,
  type AppImportItem,
  type AppImportSession,
} from '../queries/appImport'
import { formatMemory, sourceLabel, verdictVariant } from './appImportFormat'
import { AppImportConnect } from './AppImportConnect'
import { formatBytes } from './migrationHubFormat'

const PAGE = 50

function Facts({ item }: { item: AppImportItem }) {
  const { t } = useTranslation('migration')
  const e = item.entry
  const facts: string[] = []
  if (e.maps_to)
    facts.push(
      t(
        `appImport.inventory.maps.${e.maps_to as 'dockerfile' | 'railpack' | 'static' | 'image'}`,
      ),
    )
  if (e.port) facts.push(t('appImport.inventory.port', { port: e.port }))
  if (e.health?.path)
    facts.push(t('appImport.inventory.health', { path: e.health.path }))
  if (e.memory_bytes)
    facts.push(
      t('appImport.inventory.memory', { size: formatMemory(e.memory_bytes) }),
    )
  if (e.server) facts.push(t('appImport.inventory.server', { name: e.server }))
  if (e.verdict !== 'unsupported') {
    facts.push(
      t('appImport.inventory.env', {
        plain: e.env.plain,
        secret: e.env.secret,
      }),
    )
  }
  return <p className="text-xs text-muted-foreground">{facts.join(' | ')}</p>
}

function Row({
  item,
  locked,
  onToggle,
}: {
  item: AppImportItem
  locked: boolean
  onToggle: (v: boolean) => void
}) {
  const { t } = useTranslation('migration')
  const e = item.entry
  const unsupported = e.verdict === 'unsupported'
  return (
    <li className="rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-start gap-3">
        <Checkbox
          aria-label={t('appImport.inventory.selectOne', { name: item.name })}
          checked={item.selected}
          disabled={locked || unsupported}
          onCheckedChange={(v) => onToggle(v === true)}
          className="mt-0.5"
        />
        <div className="min-w-0 flex-1 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{item.name}</span>
            <Badge variant={verdictVariant[e.verdict]}>
              {t(`appImport.verdict.${e.verdict}`)}
            </Badge>
            <Badge variant="outline">
              {t(`appImport.inventory.source.${e.source}`, {
                defaultValue: e.source,
              })}
            </Badge>
          </div>
          <p className="break-all font-mono text-xs">{sourceLabel(item)}</p>
          <Facts item={item} />
          {e.domains && e.domains.length > 0 ? (
            <p className="text-xs">
              {t('appImport.inventory.domains', {
                list: e.domains.join(', '),
              })}
            </p>
          ) : null}
          {e.volumes && e.volumes.length > 0 ? (
            <ul className="text-xs text-muted-foreground">
              {e.volumes.map((v) => (
                <li key={`${v.name}-${v.container_path}`}>
                  {t('appImport.inventory.volume', {
                    path: v.container_path,
                    size: v.size_known
                      ? formatBytes(v.size_bytes)
                      : t('appImport.inventory.sizeUnknown'),
                  })}
                </li>
              ))}
            </ul>
          ) : null}
          {e.databases && e.databases.length > 0 ? (
            <p className="text-xs">
              {e.databases
                .map((d) =>
                  d.target
                    ? t('appImport.inventory.dbMoved', {
                        name: d.name,
                        target: d.target,
                      })
                    : t('appImport.inventory.dbNotMoved', { name: d.name }),
                )
                .join('; ')}
            </p>
          ) : null}
        </div>
      </div>
      {e.findings && e.findings.length > 0 ? (
        <ul className="mt-2 space-y-1 border-t pt-2 text-xs">
          {e.findings.map((f) => (
            <li key={f.reason}>
              <span>{f.reason}</span>
              {f.next ? (
                <span className="block text-muted-foreground">
                  {t('appImport.inventory.next', { action: f.next })}
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
    </li>
  )
}

interface Group {
  key: string
  project: string
  environment: string
  items: AppImportItem[]
}

function groupItems(items: AppImportItem[]): Group[] {
  const map = new Map<string, Group>()
  for (const it of items) {
    const project = it.entry.project ?? ''
    const environment = it.entry.environment ?? ''
    const key = `${project}\u0000${environment}`
    const g = map.get(key) ?? { key, project, environment, items: [] }
    g.items.push(it)
    map.set(key, g)
  }
  return [...map.values()].sort((a, b) => a.key.localeCompare(b.key))
}

function GroupSection({
  group,
  locked,
  onToggle,
}: {
  group: Group
  locked: boolean
  onToggle: (id: string, v: boolean) => void
}) {
  const { t } = useTranslation('migration')
  const [shown, setShown] = useState(PAGE)
  return (
    <section className="space-y-2">
      <h4 className="text-sm font-semibold">
        {group.project || t('appImport.inventory.noProject')}
        {group.environment ? (
          <span className="font-normal text-muted-foreground">
            {' / '}
            {group.environment}
          </span>
        ) : null}
      </h4>
      <ul className="space-y-2">
        {group.items.slice(0, shown).map((it) => (
          <Row
            key={it.source_id}
            item={it}
            locked={locked}
            onToggle={(v) => onToggle(it.source_id, v)}
          />
        ))}
      </ul>
      {group.items.length > shown ? (
        <Button
          size="sm"
          variant="outline"
          onClick={() => setShown(shown + PAGE)}
        >
          {t('appImport.inventory.showMore', {
            count: group.items.length - shown,
          })}
        </Button>
      ) : null}
    </section>
  )
}

export function AppImportInventory({
  session,
  onNext,
}: {
  session: AppImportSession
  onNext: () => void
}) {
  const { t } = useTranslation('migration')
  const put = usePutAppImportPlan()
  const groups = useMemo(() => groupItems(session.items), [session.items])
  const locked =
    session.running ||
    session.items.some(
      (i) =>
        i.state !== 'planned' &&
        i.state !== 'rolled-back' &&
        i.state !== 'stage-failed',
    )
  const selected = session.items
    .filter((i) => i.selected)
    .map((i) => i.source_id)

  function toggle(id: string, on: boolean) {
    const next = on ? [...selected, id] : selected.filter((s) => s !== id)
    put.mutate({ id: session.id, update: { selected: next } })
  }

  function selectAll(on: boolean) {
    const ids = on
      ? session.items
          .filter((i) => i.entry.verdict !== 'unsupported')
          .map((i) => i.source_id)
      : []
    put.mutate({ id: session.id, update: { selected: ids } })
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {t('appImport.inventory.title')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('appImport.inventory.description')}
        </p>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-sm">
        {(
          [
            'ready',
            'ready-with-notes',
            'needs-attention',
            'unsupported',
          ] as const
        ).map((v) => (
          <Badge key={v} variant={verdictVariant[v]}>
            {t(`appImport.verdict.${v}`)}: {session.counts[v] ?? 0}
          </Badge>
        ))}
        <Button
          size="sm"
          variant="ghost"
          disabled={locked || put.isPending}
          onClick={() => selectAll(true)}
        >
          {t('appImport.inventory.selectAll')}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={locked || put.isPending}
          onClick={() => selectAll(false)}
        >
          {t('appImport.inventory.selectNone')}
        </Button>
      </div>
      {!session.connected ? (
        <Alert>
          <AlertDescription>
            {t('appImport.inventory.stale')}
            <div className="mt-3">
              <AppImportConnect
                reconnect={{
                  id: session.id,
                  url: session.source_url,
                  platform: session.platform,
                }}
                onCreated={() => undefined}
              />
            </div>
          </AlertDescription>
        </Alert>
      ) : null}
      {put.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{put.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {groups.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('appImport.inventory.empty')}
        </p>
      ) : null}
      {groups.map((g) => (
        <GroupSection key={g.key} group={g} locked={locked} onToggle={toggle} />
      ))}
      <div className="flex items-center gap-3">
        <Button disabled={selected.length === 0} onClick={onNext}>
          {t('appImport.inventory.continue', { count: selected.length })}
        </Button>
        {selected.length === 0 ? (
          <span className="text-xs text-muted-foreground">
            {t('appImport.inventory.pickOne')}
          </span>
        ) : null}
      </div>
    </div>
  )
}
