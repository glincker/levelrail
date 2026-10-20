import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import {
  useClearLogPipeline,
  useLogPipeline,
  usePreviewLogPipeline,
  useSaveLogPipeline,
} from '../../queries/logPipeline'
import { MAX_LOG_RULES, type LogRule } from '../../types/logPipeline'
import { LogRuleRow } from './LogRuleRow'

const PREVIEW_DEBOUNCE_MS = 400

/** LogPipelinePanel edits an app's ordered log rules with a live sample preview. */
export function LogPipelinePanel({ app }: { app: string }) {
  const { t } = useTranslation('logPipeline')
  const pipeline = useLogPipeline(app)
  const save = useSaveLogPipeline(app)
  const clear = useClearLogPipeline(app)
  const preview = usePreviewLogPipeline(app)
  const [rules, setRules] = useState<LogRule[]>([])
  const [samples, setSamples] = useState('')
  const seeded = useRef(false)

  // Seed the draft once from the server; later edits stay local until saved.
  useEffect(() => {
    if (pipeline.data && !seeded.current) {
      seeded.current = true
      setRules(pipeline.data.rules)
    }
  }, [pipeline.data])

  const { mutate: runPreview, reset: resetPreview } = preview
  useEffect(() => {
    const lines = samples.split('\n').filter((l) => l !== '')
    if (lines.length === 0) {
      resetPreview()
      return
    }
    const id = setTimeout(() => {
      runPreview({ rules, lines })
    }, PREVIEW_DEBOUNCE_MS)
    return () => {
      clearTimeout(id)
    }
  }, [rules, samples, runPreview, resetPreview])

  function update(i: number, next: LogRule) {
    setRules((rs) => rs.map((r, j) => (j === i ? next : r)))
  }
  function move(i: number, delta: -1 | 1) {
    setRules((rs) => {
      const out = [...rs]
      const [r] = out.splice(i, 1)
      if (r) out.splice(i + delta, 0, r)
      return out
    })
  }
  function fail(e: Error) {
    toast.add({
      title: t('actions.failed'),
      description: e.message,
      type: 'error',
    })
  }
  function onSave() {
    save.mutate(rules, {
      onSuccess: () => {
        toast.add({ title: t('actions.savedToast'), type: 'success' })
      },
      onError: fail,
    })
  }
  function onClear() {
    clear.mutate(undefined, {
      onSuccess: () => {
        setRules([])
        toast.add({ title: t('actions.clearedToast'), type: 'success' })
      },
      onError: fail,
    })
  }

  const atLimit = rules.length >= MAX_LOG_RULES
  const busy = save.isPending || clear.isPending

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('subtitle')}</p>
      {pipeline.error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('loadError', { message: pipeline.error.message })}
          </AlertDescription>
        </Alert>
      ) : null}
      <Card>
        <CardHeader>
          <CardTitle>{t('rules.title')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {pipeline.isPending ? (
            <ListSkeleton rows={2} />
          ) : rules.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('rules.empty')}</p>
          ) : (
            <ol className="space-y-3">
              {rules.map((r, i) => (
                <LogRuleRow
                  // Rules have no id and are positional, so index is the identity.
                  key={i}
                  rule={r}
                  index={i}
                  count={rules.length}
                  onChange={(next) => {
                    update(i, next)
                  }}
                  onMove={(d) => {
                    move(i, d)
                  }}
                  onRemove={() => {
                    setRules((rs) => rs.filter((_, j) => j !== i))
                  }}
                />
              ))}
            </ol>
          )}
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              variant="outline"
              disabled={atLimit}
              onClick={() => {
                setRules((rs) => [...rs, { kind: 'redact', pattern: '' }])
              }}
            >
              {t('rules.add')}
            </Button>
            {atLimit ? (
              <span className="text-xs text-muted-foreground">
                {t('rules.limit', { max: MAX_LOG_RULES })}
              </span>
            ) : null}
            <div className="ml-auto flex gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={busy}
                onClick={onClear}
              >
                {t('actions.clear')}
              </Button>
              <Button type="button" disabled={busy} onClick={onSave}>
                {t('actions.save')}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>{t('preview.title')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <label className="block space-y-1 text-xs text-muted-foreground">
            {t('preview.samples')}
            <Textarea
              value={samples}
              rows={4}
              className="font-mono"
              placeholder={t('preview.samplesPlaceholder')}
              onChange={(e) => {
                setSamples(e.target.value)
              }}
            />
          </label>
          {preview.error ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('preview.error', { message: preview.error.message })}
              </AlertDescription>
            </Alert>
          ) : null}
          {!preview.data && !preview.error ? (
            <p className="text-sm text-muted-foreground">
              {t('preview.empty')}
            </p>
          ) : null}
          {preview.data ? (
            <ul className="divide-y divide-border">
              {preview.data.lines.map((l, i) => (
                <li key={i} className="space-y-1 py-2 text-sm">
                  <div className="flex items-start gap-2">
                    <Badge variant={l.dropped ? 'destructive' : 'muted'}>
                      {l.dropped
                        ? t('preview.dropped')
                        : t('preview.unchanged')}
                    </Badge>
                    <span
                      className={
                        l.dropped
                          ? 'min-w-0 break-all font-mono text-muted-foreground line-through'
                          : 'min-w-0 break-all font-mono text-foreground'
                      }
                    >
                      {l.message}
                    </span>
                  </div>
                  {l.fields && Object.keys(l.fields).length > 0 ? (
                    <div className="flex flex-wrap gap-1">
                      {Object.entries(l.fields).map(([k, v]) => (
                        <Badge key={k} variant="muted" className="font-mono">
                          {k}={v}
                        </Badge>
                      ))}
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
