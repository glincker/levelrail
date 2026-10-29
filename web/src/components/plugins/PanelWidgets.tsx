import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleQuestionMarkGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { StatusBadge } from '@/components/ui/status-badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { WidgetView } from '../../queries/pluginPanels'
import { SafeMarkdown } from './SafeMarkdown'

const PAGE = 50

const STATUS_VIEW = {
  ok: { tone: 'success', icon: CircleCheckGlassIcon },
  warning: { tone: 'warning', icon: TriangleAlertGlassIcon },
  error: { tone: 'danger', icon: CircleAlertGlassIcon },
  unknown: { tone: 'neutral', icon: CircleQuestionMarkGlassIcon },
} as const

function useShowMore(total: number) {
  const [shown, setShown] = useState(PAGE)
  return {
    shown,
    more: total > shown,
    showMore: () => setShown((n) => n + PAGE),
  }
}

function ShowMore({
  total,
  shown,
  onMore,
}: Readonly<{ total: number; shown: number; onMore: () => void }>) {
  const { t } = useTranslation('plugins')
  if (total <= shown) return null
  return (
    <Button variant="ghost" size="sm" onClick={onMore}>
      {t('panelHost.showMore', { count: total - shown })}
    </Button>
  )
}

function TableWidget({ w }: Readonly<{ w: WidgetView }>) {
  const { t } = useTranslation('plugins')
  const rows = w.rows ?? []
  const page = useShowMore(rows.length)
  if (rows.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('panelHost.empty')}</p>
    )
  }
  return (
    <div className="space-y-2">
      <Table>
        <TableHeader>
          <TableRow>
            {(w.columns ?? []).map((c) => (
              <TableHead key={c.key}>{c.label}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.slice(0, page.shown).map((row, i) => (
            <TableRow key={i}>
              {row.map((cell, j) => (
                <TableCell key={j} className="break-all">
                  {cell}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
      <ShowMore total={rows.length} shown={page.shown} onMore={page.showMore} />
    </div>
  )
}

function ListWidget({ w }: Readonly<{ w: WidgetView }>) {
  const { t } = useTranslation('plugins')
  const items = w.items ?? []
  const page = useShowMore(items.length)
  if (items.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('panelHost.empty')}</p>
    )
  }
  return (
    <div className="space-y-2">
      <ul className="list-disc space-y-1 pl-5 text-sm">
        {items.slice(0, page.shown).map((item, i) => (
          <li key={i} className="break-words">
            {item}
          </li>
        ))}
      </ul>
      <ShowMore
        total={items.length}
        shown={page.shown}
        onMore={page.showMore}
      />
    </div>
  )
}

function ChartWidget({ w }: Readonly<{ w: WidgetView }>) {
  const { t } = useTranslation('plugins')
  const points = (w.points ?? []).slice(0, PAGE)
  if (points.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('panelHost.empty')}</p>
    )
  }
  const top = Math.max(...points.map((p) => Math.abs(p.value)), 1)
  const rowH = 22
  return (
    <svg
      role="img"
      aria-label={w.title}
      viewBox={`0 0 100 ${points.length * rowH}`}
      preserveAspectRatio="none"
      className="h-auto w-full text-primary"
    >
      {points.map((p, i) => (
        <rect
          key={i}
          x={0}
          y={i * rowH + 4}
          width={(Math.abs(p.value) / top) * 100}
          height={rowH - 8}
          rx={1}
          className="fill-current"
        >
          <title>{`${p.label}: ${p.value}`}</title>
        </rect>
      ))}
    </svg>
  )
}

function ChartLegend({ w }: Readonly<{ w: WidgetView }>) {
  const points = (w.points ?? []).slice(0, PAGE)
  return (
    <dl className="mt-2 grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
      {points.map((p, i) => (
        <div key={i} className="flex justify-between gap-2">
          <dt className="truncate text-muted-foreground">{p.label}</dt>
          <dd>{p.value}</dd>
        </div>
      ))}
    </dl>
  )
}

function ButtonWidget({
  w,
  onPress,
  pending,
}: Readonly<{
  w: WidgetView
  onPress: (action: string) => Promise<void>
  pending: boolean
}>) {
  const { t } = useTranslation('plugins')
  const [confirming, setConfirming] = useState(false)
  const action = w.action ?? ''
  const run = async () => {
    await onPress(action)
    setConfirming(false)
  }
  return (
    <>
      <Button
        variant="outline"
        size="sm"
        disabled={pending}
        onClick={() => (w.confirm ? setConfirming(true) : void run())}
      >
        {w.label}
      </Button>
      {w.confirm ? (
        <ConfirmDialog
          open={confirming}
          onOpenChange={setConfirming}
          title={w.label ?? ''}
          description={w.confirm}
          confirmLabel={t('panelHost.confirm')}
          pending={pending}
          onConfirm={() => void run()}
        />
      ) : null}
    </>
  )
}

export function PanelWidget({
  w,
  onPress,
  pending,
}: Readonly<{
  w: WidgetView
  onPress: (action: string) => Promise<void>
  pending: boolean
}>) {
  const { t } = useTranslation('plugins')
  if (w.error) {
    return (
      <p className="text-sm text-muted-foreground">
        {w.title ? `${w.title}: ` : ''}
        {t('panelHost.widgetUnavailable')}
      </p>
    )
  }
  switch (w.type) {
    case 'stat':
      return (
        <div>
          <p className="text-xs text-muted-foreground">{w.title}</p>
          <p className="text-2xl font-semibold tabular-nums">
            {w.value}
            {w.unit ? (
              <span className="ml-1 text-sm font-normal text-muted-foreground">
                {w.unit}
              </span>
            ) : null}
          </p>
        </div>
      )
    case 'status': {
      const view = STATUS_VIEW[w.state ?? 'unknown']
      return (
        <div className="flex items-center gap-2 text-sm">
          <StatusBadge
            tone={view.tone}
            icon={view.icon}
            label={t(`panelHost.status.${w.state ?? 'unknown'}`)}
          />
          <span className="break-words">{w.text || w.title}</span>
        </div>
      )
    }
    case 'markdown':
      return <SafeMarkdown text={w.text ?? ''} />
    case 'table':
      return <TableWidget w={w} />
    case 'list':
      return <ListWidget w={w} />
    case 'chart':
      return (
        <div>
          <ChartWidget w={w} />
          <ChartLegend w={w} />
        </div>
      )
    case 'button':
      return <ButtonWidget w={w} onPress={onPress} pending={pending} />
    default:
      return null
  }
}
