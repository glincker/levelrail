import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { useEdgeAccess } from '../../queries/edgeAccess'

/** EdgeAccessPanel lists recent edge requests with their request and trace ids. */
export function EdgeAccessPanel({ app }: { app: string }) {
  const { t } = useTranslation('edgeAccess')
  const [minMs, setMinMs] = useState('')
  const [path, setPath] = useState('')
  const [errorsOnly, setErrorsOnly] = useState(false)
  const list = useEdgeAccess(app, {
    minMs: Number(minMs) > 0 ? Number(minMs) : 0,
    errorsOnly,
    path,
  })

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('subtitle')}</p>
      <Card>
        <CardHeader className="gap-3">
          <CardTitle>{t('title')}</CardTitle>
          <div className="flex flex-wrap items-end gap-4">
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
            <label className="space-y-1 text-xs text-muted-foreground">
              {t('filters.path')}
              <Input
                value={path}
                className="w-48"
                placeholder="/api"
                onChange={(e) => {
                  setPath(e.target.value)
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
              {list.data.map((r) => (
                <li
                  key={`${r.time}-${r.request_id}`}
                  className="grid grid-cols-[auto_minmax(0,1fr)_auto_auto] items-center gap-4 px-2 py-2 text-sm"
                >
                  <Badge variant={r.status >= 500 ? 'destructive' : 'muted'}>
                    {r.status}
                  </Badge>
                  <span className="min-w-0">
                    <span className="block truncate font-medium text-foreground">
                      <span className="mr-2 text-muted-foreground">
                        {r.method}
                      </span>
                      {r.path}
                    </span>
                    <span className="block truncate font-mono text-xs text-muted-foreground">
                      {new Date(r.time).toLocaleTimeString()} {r.request_id}
                      {r.trace_id ? ` ${r.trace_id}` : ''}
                    </span>
                  </span>
                  <span className="tabular-nums text-muted-foreground">
                    {Math.round(r.duration_ms)} ms
                  </span>
                  {r.upstream_error ? (
                    <Badge variant="destructive">{t('list.upstream')}</Badge>
                  ) : (
                    <span aria-hidden="true" className="w-12" />
                  )}
                </li>
              ))}
            </ul>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
