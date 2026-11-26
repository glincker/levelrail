import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CloudGlassIcon,
  ServerGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { formatBytes } from '../../lib/format'
import { usePlacementNodes } from './usePlacementNodes'
import {
  AUTO_NODE,
  ineligibleReason,
  pickAutoNode,
  type PlacementKind,
  type PlacementNode,
} from './placementPick'

// Cloud and datacenter panels pull in provider queries and the join flow,
// so they load only when one of those two choices is picked.
const PlacementCloud = lazy(() =>
  import('./PlacementCloud').then((m) => ({ default: m.PlacementCloud })),
)
const PlacementHost = lazy(() =>
  import('./PlacementCloud').then((m) => ({ default: m.PlacementHost })),
)
const AddNodeWizard = lazy(() =>
  import('../AddNodeWizard').then((m) => ({ default: m.AddNodeWizard })),
)

const CARD =
  'rounded-lg border border-input text-left transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none'

const KINDS: readonly {
  id: PlacementKind
  Icon: typeof ServerGlassIcon
}[] = [
  { id: 'local', Icon: ServerGlassIcon },
  { id: 'node', Icon: ServerGlassIcon },
  { id: 'cloud', Icon: CloudGlassIcon },
  { id: 'host', Icon: TerminalGlassIcon },
]

function Hint({ children }: Readonly<{ children: React.ReactNode }>) {
  return <p className="text-xs text-muted-foreground">{children}</p>
}

export function Placement({
  kind,
  nodeId,
  onKind,
  onNode,
}: Readonly<{
  kind: PlacementKind
  nodeId: string
  onKind: (kind: PlacementKind) => void
  onNode: (id: string) => void
}>) {
  const { t } = useTranslation('databases')
  const nodes = usePlacementNodes()
  const local = nodes.find((n) => n.isLocal)
  const others = nodes.filter((n) => !n.isLocal)
  const auto = pickAutoNode(nodes)
  const chosen = nodes.find((n) => n.id === nodeId)

  const summary =
    kind === 'local'
      ? t('flow.placement.summary.local', {
          name: local?.name ?? t('flow.placement.kinds.local.title'),
        })
      : kind === 'node'
        ? nodeId === AUTO_NODE
          ? auto
            ? t('flow.placement.summary.auto', { name: auto.name })
            : t('flow.placement.autoNone')
          : t('flow.placement.summary.node', { name: chosen?.name ?? '' })
        : t(`flow.placement.summary.${kind}`)

  return (
    <section className="min-w-0 space-y-2">
      <h3 className="text-sm font-medium">{t('flow.placement.title')}</h3>
      <div
        role="radiogroup"
        aria-label={t('flow.placement.title')}
        className="grid min-w-0 grid-cols-1 gap-2 min-[480px]:grid-cols-2"
      >
        {KINDS.map(({ id, Icon }) => (
          <button
            key={id}
            type="button"
            role="radio"
            aria-checked={kind === id}
            onClick={() => {
              onKind(id)
            }}
            className={cn(
              CARD,
              'flex min-w-0 items-center gap-3 p-2.5',
              kind === id && 'border-primary bg-muted',
            )}
          >
            <Icon aria-hidden="true" className="size-6 shrink-0" />
            <span className="min-w-0">
              <span className="block truncate text-sm font-medium">
                {t(`flow.placement.kinds.${id}.title`)}
              </span>
              <span className="block truncate text-xs text-muted-foreground">
                {t(`flow.placement.kinds.${id}.note`)}
              </span>
            </span>
          </button>
        ))}
      </div>

      {kind === 'local' ? <LocalDetail node={local} /> : null}
      {kind === 'node' ? (
        <ExistingNodes
          others={others}
          all={nodes}
          nodeId={nodeId}
          onNode={onNode}
        />
      ) : null}
      <Suspense fallback={null}>
        {kind === 'cloud' ? <PlacementCloud /> : null}
        {kind === 'host' ? <PlacementHost /> : null}
      </Suspense>
      <p
        data-testid="placement-summary"
        className="text-xs text-muted-foreground"
      >
        {summary}
      </p>
    </section>
  )
}

function LocalDetail({ node }: Readonly<{ node?: PlacementNode }>) {
  const { t } = useTranslation('databases')
  return (
    <Hint>
      {node?.diskFreeBytes === undefined
        ? t('flow.placement.localNoDisk', {
            name: node?.name ?? t('flow.placement.kinds.local.title'),
          })
        : t('flow.placement.local', {
            name: node.name,
            free: formatBytes(node.diskFreeBytes),
          })}
    </Hint>
  )
}

export function ExistingNodes({
  others,
  all,
  nodeId,
  onNode,
}: Readonly<{
  others: readonly PlacementNode[]
  all: readonly PlacementNode[]
  nodeId: string
  onNode: (id: string) => void
}>) {
  const { t } = useTranslation('databases')
  const auto = pickAutoNode(all)
  const nodeDetail = (n: PlacementNode): string => {
    const parts = [n.zone ?? n.region].filter(Boolean) as string[]
    parts.push(
      n.diskFreeBytes === undefined
        ? t('flow.placement.diskUnknown')
        : t('flow.placement.diskFree', { free: formatBytes(n.diskFreeBytes) }),
    )
    if (n.cpuPercent !== undefined) {
      parts.push(
        t('flow.placement.load', { percent: Math.round(n.cpuPercent) }),
      )
    }
    parts.push(t('flow.placement.workloads', { count: n.workloads }))
    return parts.join(' / ')
  }
  if (others.length === 0) {
    return (
      <div className="flex min-w-0 flex-wrap items-center justify-between gap-2 rounded-lg border border-dashed border-border p-2.5">
        <Hint>{t('flow.placement.noNodes')}</Hint>
        <Suspense fallback={null}>
          <AddNodeWizard triggerLabel={t('flow.placement.addNode')} />
        </Suspense>
      </div>
    )
  }
  return (
    <div
      role="radiogroup"
      aria-label={t('flow.placement.kinds.node.title')}
      className="min-w-0 space-y-1.5"
    >
      <NodeRow
        selected={nodeId === AUTO_NODE}
        onSelect={() => {
          onNode(AUTO_NODE)
        }}
        title={t('flow.placement.auto')}
        detail={
          auto
            ? t('flow.placement.autoPick', { name: auto.name })
            : t('flow.placement.autoNone')
        }
      />
      {all.map((n) => {
        const reason = ineligibleReason(n)
        return (
          <NodeRow
            key={n.id}
            selected={nodeId === n.id}
            disabled={reason !== null}
            onSelect={() => {
              onNode(n.id)
            }}
            title={n.name}
            detail={nodeDetail(n)}
            reason={reason ? t(`flow.placement.reasons.${reason}`) : undefined}
          />
        )
      })}
      <Suspense fallback={null}>
        <AddNodeWizard triggerLabel={t('flow.placement.addNode')} />
      </Suspense>
    </div>
  )
}

function NodeRow({
  selected,
  disabled = false,
  onSelect,
  title,
  detail,
  reason,
}: Readonly<{
  selected: boolean
  disabled?: boolean
  onSelect: () => void
  title: string
  detail: string
  reason?: string
}>) {
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      disabled={disabled}
      onClick={onSelect}
      className={cn(
        CARD,
        'flex w-full min-w-0 items-center justify-between gap-3 px-3 py-2 disabled:cursor-not-allowed disabled:opacity-60',
        selected && 'border-primary bg-muted',
      )}
    >
      <span className="truncate text-sm font-medium">{title}</span>
      <span className="min-w-0 truncate text-xs text-muted-foreground">
        {reason ?? detail}
      </span>
    </button>
  )
}
