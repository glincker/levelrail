import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { InfoTip } from '../kit'
import {
  useDeleteObservabilityExport,
  useExportPresets,
  useObservabilityExports,
  useTestObservabilityExport,
  type ExportPreset,
  type ExportTestResult,
  type ObservabilityExport,
} from '../../queries/observabilityExports'
import { PresetGlyph } from './presetBrand'
import { TestResults } from './TestResults'

// The dialog and its field rendering only load once a preset is picked.
const ExportPresetDialog = lazy(() =>
  import('./ExportPresetDialog').then((m) => ({
    default: m.ExportPresetDialog,
  })),
)

type Health = 'ok' | 'error' | 'pending'

function exportHealth(exp: ObservabilityExport): Health {
  const active = exp.signals.filter((s) => s.enabled)
  if (active.some((s) => s.circuit_open || s.last_error)) return 'error'
  if (active.length > 0 && active.every((s) => s.last_success_at)) return 'ok'
  return 'pending'
}

const DOT_CLASS: Record<Health, string> = {
  ok: 'bg-tone-success',
  error: 'bg-tone-danger',
  pending: 'bg-muted-foreground/50',
}

function ExportRow({ exp }: Readonly<{ exp: ObservabilityExport }>) {
  const { t } = useTranslation('settings')
  const test = useTestObservabilityExport()
  const remove = useDeleteObservabilityExport()
  const [result, setResult] = useState<ExportTestResult | null>(null)
  const health = exportHealth(exp)
  const sent = exp.signals.reduce((n, s) => n + s.sent_total, 0)

  return (
    <li className="space-y-2 py-2">
      <div className="flex flex-wrap items-center gap-3">
        <span
          role="img"
          aria-label={t(`obsExport.health.${health}`)}
          title={t(`obsExport.health.${health}`)}
          className={`size-2.5 shrink-0 rounded-full ${DOT_CLASS[health]}`}
        />
        <PresetGlyph id={exp.preset} />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {exp.preset_name}
          <span className="ml-2 text-xs font-normal text-muted-foreground">
            {exp.signals
              .map((s) => t(`obsExport.signal.${s.signal}`))
              .join(', ')}
          </span>
        </span>
        <span className="text-xs text-muted-foreground tabular-nums">
          {t('obsExport.list.sent', { count: sent })}
        </span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={test.isPending}
          onClick={() => test.mutate(exp.id, { onSuccess: setResult })}
        >
          {test.isPending
            ? t('obsExport.test.running')
            : t('obsExport.test.run')}
        </Button>
        <ConfirmAction
          trigger={(open) => (
            <Button type="button" size="sm" variant="outline" onClick={open}>
              {t('obsExport.list.delete')}
            </Button>
          )}
          title={t('obsExport.list.deleteTitle', { name: exp.preset_name })}
          consequences={[t('obsExport.list.deleteConsequence')]}
          confirmLabel={t('obsExport.list.delete')}
          pending={remove.isPending}
          error={remove.error?.message ?? null}
          onConfirm={(close) => remove.mutate(exp.id, { onSuccess: close })}
          onOpenChange={() => remove.reset()}
        />
      </div>
      {exp.signals.find((s) => s.last_error) ? (
        <p className="pl-5 text-xs text-tone-danger">
          {exp.signals.find((s) => s.last_error)?.last_error}
        </p>
      ) : null}
      {test.error ? (
        <p role="alert" className="pl-5 text-xs text-tone-danger">
          {test.error.message}
        </p>
      ) : null}
      {result ? <TestResults result={result} /> : null}
    </li>
  )
}

export function ObservabilityExportSection() {
  const { t } = useTranslation('settings')
  const presets = useExportPresets()
  const exports = useObservabilityExports()
  const [picked, setPicked] = useState<ExportPreset | null>(null)

  // A build without the catalog (501 or older server) hides the section.
  if (presets.isError || !presets.data) return null
  const list = exports.data ?? []

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-1.5">
          {t('obsExport.title')}
          <InfoTip label={t('obsExport.title')}>{t('obsExport.about')}</InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <ul
          className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3"
          aria-label={t('obsExport.title')}
        >
          {presets.data.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => setPicked(p)}
                className="flex w-full items-center gap-2.5 rounded-lg border p-2.5 text-left transition-colors hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none"
              >
                <PresetGlyph id={p.id} />
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">
                    {p.name}
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {p.description}
                  </span>
                </span>
              </button>
            </li>
          ))}
        </ul>
        {list.length > 0 ? (
          <ul className="divide-y" aria-label={t('obsExport.list.label')}>
            {list.map((exp) => (
              <ExportRow key={exp.id} exp={exp} />
            ))}
          </ul>
        ) : null}
      </CardContent>
      {picked ? (
        <Suspense fallback={null}>
          <ExportPresetDialog preset={picked} onClose={() => setPicked(null)} />
        </Suspense>
      ) : null}
    </Card>
  )
}
