import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Share2GlassIcon } from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  circleLayout,
  edgeKey,
  edgeTone,
  MAP_SIZE,
  type EdgeTone,
} from '../../../lib/privateNetworkLayout'
import type { PrivateNetworkMap as MapData } from '../../../queries/privateNetworks'
import { PrivatePathsList } from './PrivatePathsList'

const TONE_CLASS: Record<EdgeTone, string> = {
  allowed: 'stroke-primary',
  blocked: 'stroke-destructive [stroke-dasharray:5_4]',
  drift: 'stroke-amber-500 [stroke-dasharray:2_3]',
}

const TONE_LEGEND = [
  { tone: 'allowed', key: 'map.legendAllowed' },
  { tone: 'blocked', key: 'map.legendBlocked' },
  { tone: 'drift', key: 'map.legendDrift' },
] as const

export function PrivateNetworkMap({ data }: { data: MapData }) {
  const { t } = useTranslation('privateNetworks')
  const [active, setActive] = useState<string | null>(null)
  const points = useMemo(
    () =>
      new Map(circleLayout(data.apps.map((a) => a.id)).map((p) => [p.id, p])),
    [data.apps],
  )
  const names = useMemo(
    () => new Map(data.apps.map((a) => [a.id, a.name])),
    [data.apps],
  )

  if (data.apps.length === 0) {
    return (
      <EmptyState
        icon={<Share2GlassIcon className="size-6" />}
        title={t('map.empty')}
        description={t('map.emptyHint')}
      />
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('map.title')}</CardTitle>
        <CardDescription>{t('map.description')}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4 lg:grid-cols-[minmax(0,22rem)_1fr]">
        <div>
          <svg
            viewBox={`0 0 ${MAP_SIZE} ${MAP_SIZE}`}
            role="img"
            aria-label={t('map.graphLabel')}
            className="w-full max-w-sm"
          >
            {data.edges.map((e) => {
              const a = points.get(e.a)
              const b = points.get(e.b)
              if (!a || !b) return null
              const key = edgeKey(e)
              return (
                <line
                  key={key}
                  x1={a.x}
                  y1={a.y}
                  x2={b.x}
                  y2={b.y}
                  strokeWidth={active === key ? 4 : 2}
                  className={TONE_CLASS[edgeTone(e)]}
                  opacity={active && active !== key ? 0.25 : 1}
                />
              )
            })}
            {data.apps.map((app) => {
              const p = points.get(app.id)
              if (!p) return null
              return (
                <g key={app.id}>
                  <circle
                    cx={p.x}
                    cy={p.y}
                    r={9}
                    className="fill-card stroke-foreground"
                    strokeWidth={2}
                  />
                  <text
                    x={p.x}
                    y={p.y + 24}
                    textAnchor="middle"
                    className="fill-foreground text-[10px]"
                  >
                    {app.name}
                  </text>
                </g>
              )
            })}
          </svg>
          <ul className="mt-2 flex flex-wrap gap-3 text-xs text-muted-foreground">
            {TONE_LEGEND.map(({ tone, key }) => (
              <li key={tone} className="flex items-center gap-1.5">
                <svg width="20" height="6" aria-hidden="true">
                  <line
                    x1="0"
                    y1="3"
                    x2="20"
                    y2="3"
                    strokeWidth={2}
                    className={TONE_CLASS[tone]}
                  />
                </svg>
                {t(key)}
              </li>
            ))}
          </ul>
        </div>
        <PrivatePathsList
          edges={data.edges}
          names={names}
          onActive={setActive}
        />
      </CardContent>
    </Card>
  )
}
