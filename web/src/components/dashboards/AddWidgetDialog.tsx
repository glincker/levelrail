import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  isDashboardMetric,
  isHttpUrl,
  metricsFor,
  newWidgetId,
  WIDGET_TARGETS,
  WIDGET_TYPES,
} from '@/lib/dashboardGrid'
import { useAppListOptional } from '@/queries/apps'
import { useDatabases } from '@/queries/databases'
import { useNodeListOptional } from '@/queries/nodes'
import type {
  DashboardWidget,
  TargetKind,
  WidgetType,
} from '@/types/dashboards'

function useTargetNames(kind: TargetKind | undefined): string[] {
  const apps = useAppListOptional()
  const nodes = useNodeListOptional()
  const dbs = useDatabases()
  if (kind === 'app') return (apps.data ?? []).map((a) => a.name)
  if (kind === 'node') return (nodes.data ?? []).map((n) => n.name)
  if (kind === 'database') return (dbs.data ?? []).map((d) => d.name)
  return []
}

/** Picks a widget type, what it reads from, and a metric or log filter where that applies. */
export function AddWidgetDialog({
  open,
  onOpenChange,
  existing,
  onAdd,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  existing: readonly DashboardWidget[]
  onAdd: (w: DashboardWidget) => void
}) {
  const { t } = useTranslation('dashboards')
  const [type, setType] = useState<WidgetType>('metric_chart')
  const kinds = WIDGET_TARGETS[type]
  const [pickedKind, setPickedKind] = useState<TargetKind>('app')
  const kind = kinds
    ? kinds.includes(pickedKind)
      ? pickedKind
      : kinds[0]
    : undefined
  const [name, setName] = useState('')
  const [metric, setMetric] = useState('cpu_percent')
  const [query, setQuery] = useState('')
  const [linkTitle, setLinkTitle] = useState('')
  const [linkUrl, setLinkUrl] = useState('')
  const names = useTargetNames(kind)
  const needsTarget = kind !== undefined
  const isLink = type === 'link'
  const canAdd = isLink
    ? linkTitle.trim() !== '' && isHttpUrl(linkUrl.trim())
    : !needsTarget || name !== ''

  function submit() {
    const w: DashboardWidget = {
      id: newWidgetId(existing, type),
      type,
      size: type === 'log_tail' ? 'L' : isLink ? 'S' : 'M',
    }
    if (isLink) {
      w.title = linkTitle.trim()
      w.url = linkUrl.trim()
    }
    if (kind) w.target = { kind, name }
    if (type === 'metric_chart') {
      w.metric =
        kind && metricsFor(kind).includes(metric) ? metric : 'cpu_percent'
    }
    if (type === 'log_tail' && query.trim()) w.query = query.trim()
    onAdd(w)
    onOpenChange(false)
    setName('')
    setQuery('')
    setLinkTitle('')
    setLinkUrl('')
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t('add.title')}</DialogTitle>
          <DialogDescription>{t('add.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="widget-type">{t('add.type')}</Label>
            <Select
              value={type}
              onValueChange={(v) => {
                if (v) {
                  setType(v)
                  setName('')
                }
              }}
            >
              <SelectTrigger id="widget-type" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WIDGET_TYPES.map((wt) => (
                  <SelectItem key={wt} value={wt}>
                    {t(`widget.type.${wt}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {kinds && kinds.length > 1 ? (
            <div className="space-y-1.5">
              <Label htmlFor="widget-kind">{t('add.kind')}</Label>
              <Select
                value={kind}
                onValueChange={(v) => {
                  if (v) {
                    setPickedKind(v)
                    setName('')
                  }
                }}
              >
                <SelectTrigger id="widget-kind" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {kinds.map((k) => (
                    <SelectItem key={k} value={k}>
                      {t(`kind.${k}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          ) : null}
          {needsTarget ? (
            <div className="space-y-1.5">
              <Label htmlFor="widget-target">{t('add.target')}</Label>
              <Select
                value={name}
                onValueChange={(v) => {
                  if (v) setName(v)
                }}
              >
                <SelectTrigger id="widget-target" className="w-full">
                  <SelectValue placeholder={t('add.targetPlaceholder')} />
                </SelectTrigger>
                <SelectContent>
                  {names.map((n) => (
                    <SelectItem key={n} value={n}>
                      {n}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          ) : null}
          {type === 'metric_chart' && kind ? (
            <div className="space-y-1.5">
              <Label htmlFor="widget-metric">{t('add.metric')}</Label>
              <Select
                value={metric}
                onValueChange={(v) => {
                  if (v) setMetric(v)
                }}
              >
                <SelectTrigger id="widget-metric" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {metricsFor(kind).map((m) => (
                    <SelectItem key={m} value={m}>
                      {isDashboardMetric(m) ? t(`metric.${m}`) : m}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          ) : null}
          {isLink ? (
            <>
              <div className="space-y-1.5">
                <Label htmlFor="widget-link-title">{t('add.linkTitle')}</Label>
                <Input
                  id="widget-link-title"
                  value={linkTitle}
                  maxLength={80}
                  onChange={(e) => setLinkTitle(e.target.value)}
                  placeholder={t('add.linkTitlePlaceholder')}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="widget-link-url">{t('add.linkUrl')}</Label>
                <Input
                  id="widget-link-url"
                  type="url"
                  value={linkUrl}
                  maxLength={2048}
                  onChange={(e) => setLinkUrl(e.target.value)}
                  placeholder="https://"
                />
              </div>
            </>
          ) : null}
          {type === 'log_tail' ? (
            <div className="space-y-1.5">
              <Label htmlFor="widget-query">{t('add.query')}</Label>
              <Input
                id="widget-query"
                value={query}
                maxLength={256}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t('add.queryPlaceholder')}
              />
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button disabled={!canAdd} onClick={submit}>
            {t('add.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
