import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { formatDuration } from '../../lib/traceTree'
import { useTraces } from '../../queries/traces'
import { TraceWaterfall } from './TraceWaterfall'
import { TracingSetupCard } from './TracingSetupCard'

const WINDOWS = ['15', '60', '360', '1440'] as const

/** TracesPanel is the app's Traces section: setup, filters, list, waterfall. */
export function TracesPanel({ app }: { app: string }) {
  const { t } = useTranslation('traces')
  const [windowMinutes, setWindowMinutes] = useState<string>('60')
  const [errorsOnly, setErrorsOnly] = useState(false)
  const [minMs, setMinMs] = useState('')
  const [selected, setSelected] = useState<string | null>(null)
  const list = useTraces(app, {
    errorsOnly,
    minMs: Number(minMs) > 0 ? Number(minMs) : 0,
    windowMinutes: Number(windowMinutes),
  })

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('subtitle')}</p>
      <TracingSetupCard app={app} />

      <Card>
        <CardHeader className="gap-3">
          <CardTitle>{t('title')}</CardTitle>
          <div className="flex flex-wrap items-end gap-4">
            <label className="space-y-1 text-xs text-muted-foreground">
              {t('filters.window')}
              <Select
                value={windowMinutes}
                onValueChange={(v) => {
                  if (typeof v === 'string') setWindowMinutes(v)
                }}
              >
                <SelectTrigger className="w-44">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {WINDOWS.map((w) => (
                    <SelectItem key={w} value={w}>
                      {t(`filters.last${w}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <label className="space-y-1 text-xs text-muted-foreground">
              {t('filters.minMs')}
              <Input
                type="number"
                min={0}
                value={minMs}
                className="w-32"
                onChange={(e) => {
                  setMinMs(e.target.value)
                }}
              />
            </label>
            <label className="flex items-center gap-2 pb-2 text-sm text-foreground">
              <Switch checked={errorsOnly} onCheckedChange={setErrorsOnly} />
              {t('filters.errorsOnly')}
            </label>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {list.error ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('list.loadError', { message: list.error.message })}
              </AlertDescription>
            </Alert>
          ) : null}
          {list.data && list.data.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('list.empty')}</p>
          ) : null}
          {list.data && list.data.length > 0 ? (
            <ul className="divide-y divide-border">
              {list.data.map((tr) => (
                <li key={tr.trace_id}>
                  <button
                    type="button"
                    onClick={() => {
                      setSelected((cur) =>
                        cur === tr.trace_id ? null : tr.trace_id,
                      )
                    }}
                    aria-expanded={selected === tr.trace_id}
                    className={cn(
                      'grid w-full grid-cols-[minmax(0,1fr)_auto_auto_auto] items-center gap-4 px-2 py-2 text-left text-sm hover:bg-muted/60',
                      selected === tr.trace_id && 'bg-muted/60',
                    )}
                  >
                    <span className="min-w-0">
                      <span className="block truncate font-medium text-foreground">
                        {tr.root_name}
                      </span>
                      <span className="block text-xs text-muted-foreground">
                        {new Date(tr.start_ns / 1_000_000).toLocaleTimeString()}{' '}
                        <span className="font-mono">
                          {tr.trace_id.slice(0, 8)}
                        </span>
                      </span>
                    </span>
                    <span className="tabular-nums text-muted-foreground">
                      {formatDuration(tr.duration_ns)}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {t('list.spans')} {tr.spans}
                    </span>
                    {tr.errors > 0 ? (
                      <Badge variant="destructive">
                        {t('detail.error')} {tr.errors}
                      </Badge>
                    ) : (
                      <span aria-hidden="true" className="w-12" />
                    )}
                  </button>
                  {selected === tr.trace_id ? (
                    <div className="px-2 pb-4 pt-2">
                      <TraceWaterfall app={app} id={tr.trace_id} />
                    </div>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
