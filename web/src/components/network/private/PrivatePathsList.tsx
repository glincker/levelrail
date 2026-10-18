import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { edgeKey, edgeTone } from '../../../lib/privateNetworkLayout'
import {
  useApplyPrivateNetworks,
  useCreatePrivateNetworkRule,
  type PrivateNetworkMapEdge,
} from '../../../queries/privateNetworks'
import { useIsRoot } from '../../../hooks/useIsRoot'

const BADGE = {
  allowed: 'success',
  blocked: 'destructive',
  drift: 'warning',
} as const

const ROW_HEIGHT = 56
const VIRTUALIZE_ABOVE = 50

interface Props {
  edges: PrivateNetworkMapEdge[]
  names: Map<string, string>
  onActive: (key: string | null) => void
}

export function PrivatePathsList({ edges, names, onActive }: Props) {
  const { t } = useTranslation('privateNetworks')
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualized = edges.length > VIRTUALIZE_ABOVE
  const virtualizer = useVirtualizer({
    count: edges.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
    enabled: virtualized,
  })

  if (edges.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('paths.none')}</p>
  }

  const row = (e: PrivateNetworkMapEdge) => (
    <PathRow key={edgeKey(e)} edge={e} names={names} onActive={onActive} />
  )

  return (
    <div
      ref={parentRef}
      className="max-h-96 overflow-auto rounded-lg border border-border"
      aria-label={t('paths.title')}
    >
      {virtualized ? (
        <ul className="relative" style={{ height: virtualizer.getTotalSize() }}>
          {virtualizer.getVirtualItems().map((v) => {
            const e = edges[v.index]
            if (!e) return null
            return (
              <li
                key={edgeKey(e)}
                className="absolute inset-x-0 border-b border-border"
                style={{
                  height: v.size,
                  transform: `translateY(${v.start}px)`,
                }}
              >
                <PathRow edge={e} names={names} onActive={onActive} bare />
              </li>
            )
          })}
        </ul>
      ) : (
        <ul className="divide-y divide-border">{edges.map(row)}</ul>
      )}
    </div>
  )
}

function PathRow({
  edge,
  names,
  onActive,
  bare = false,
}: {
  edge: PrivateNetworkMapEdge
  names: Map<string, string>
  onActive: (key: string | null) => void
  bare?: boolean
}) {
  const { t } = useTranslation('privateNetworks')
  const isRoot = useIsRoot()
  const create = useCreatePrivateNetworkRule()
  const apply = useApplyPrivateNetworks()
  const a = names.get(edge.a) ?? edge.a
  const b = names.get(edge.b) ?? edge.b
  const tone = edgeTone(edge)
  const key = edgeKey(edge)

  const status =
    edge.fix === 'reapply'
      ? t('paths.drift')
      : !edge.allowed
        ? t('paths.blocked')
        : edge.via === 'network'
          ? t('paths.allowedVia', { name: edge.via_name })
          : t('paths.allowedRule')

  function fix() {
    if (edge.fix === 'allow') {
      create.mutate(
        { from: a, to: b },
        {
          onSuccess: () =>
            toast.add({ title: t('paths.allowed', { a, b }), type: 'success' }),
          onError: (err) => toast.add({ title: err.message, type: 'error' }),
        },
      )
    } else if (edge.fix === 'reapply') {
      apply.mutate(undefined, {
        onSuccess: (r) =>
          toast.add({
            title: r.enforced ? t('page.applied') : t('page.notApplied'),
            type: r.enforced ? 'success' : 'error',
          }),
        onError: (err) => toast.add({ title: err.message, type: 'error' }),
      })
    }
  }

  const content = (
    <div
      className="flex items-center justify-between gap-3 px-3 py-2"
      onMouseEnter={() => onActive(key)}
      onMouseLeave={() => onActive(null)}
      onFocus={() => onActive(key)}
      onBlur={() => onActive(null)}
    >
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">
          {t('paths.pair', { a, b })}
        </p>
        <p className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
          <Badge variant={BADGE[tone]}>{status}</Badge>
          {edge.cross_node ? <span>{t('paths.crossNode')}</span> : null}
        </p>
      </div>
      {edge.fix && isRoot ? (
        <Button
          size="sm"
          variant="outline"
          onClick={fix}
          disabled={create.isPending || apply.isPending}
        >
          {edge.fix === 'allow' ? t('paths.fixAllow') : t('paths.fixReapply')}
        </Button>
      ) : null}
    </div>
  )
  return bare ? content : <li>{content}</li>
}
