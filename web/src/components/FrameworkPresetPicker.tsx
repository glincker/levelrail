import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CopyGlassIcon,
  SparklesGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { FieldSectionLabel } from '@/components/ui/field'
import { cn } from '@/lib/utils'
import {
  fetchFrameworkDockerfile,
  useFrameworks,
  type FrameworkPreset,
} from '../queries/frameworks'
import { presetDiffers } from '../lib/frameworkPreset'
import { BrandIcon } from './BrandIcon'

export function FrameworkPresetPicker({
  detectedId,
  port,
  healthPath,
  disabled,
  onApply,
}: {
  detectedId?: string
  port: string
  healthPath: string
  disabled: boolean
  onApply: (preset: FrameworkPreset) => void
}) {
  const { t } = useTranslation('frameworks')
  const presets = useFrameworks()
  const [picked, setPicked] = useState<string | null>(null)

  const activeId = picked ?? detectedId
  const active = presets.data?.find((p) => p.id === activeId)

  const copyDockerfile = async (preset: FrameworkPreset) => {
    try {
      await navigator.clipboard.writeText(
        await fetchFrameworkDockerfile(preset.id),
      )
      toast.add({ title: t('dockerfileCopied'), type: 'success' })
    } catch {
      toast.add({ title: t('dockerfileFailed'), type: 'error' })
    }
  }

  if (presets.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
        <AlertDescription>{t('loadFailed')}</AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="space-y-3">
      <FieldSectionLabel>{t('title')}</FieldSectionLabel>
      <p className="text-sm text-muted-foreground">{t('description')}</p>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        {(presets.data ?? []).map((p) => (
          <button
            key={p.id}
            type="button"
            aria-pressed={p.id === activeId}
            disabled={disabled}
            onClick={() => {
              setPicked(p.id)
              onApply(p)
            }}
            className={cn(
              'flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-left text-sm transition-colors hover:bg-muted/60 disabled:opacity-50',
              p.id === activeId && 'border-primary bg-muted',
            )}
          >
            <BrandIcon name={p.icon} className="size-4 shrink-0" />
            <span className="min-w-0 flex-1 truncate">{p.name}</span>
            {p.id === detectedId ? (
              <SparklesGlassIcon
                className="size-4 shrink-0 text-muted-foreground"
                aria-label={t('detected')}
              />
            ) : null}
          </button>
        ))}
      </div>

      {active ? (
        <div className="space-y-3 rounded-lg border border-border bg-muted/40 p-3 text-sm">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="muted">
              {t('buildType')}: {active.build_type}
            </Badge>
            <Badge variant="muted">
              {t('outputMode')}: {t(`outputModes.${active.output_mode}`)}
            </Badge>
            {active.build_type === 'static' ? null : (
              <>
                <Badge variant="muted">
                  {t('port')}: {active.port}
                </Badge>
                <Badge variant="muted">
                  {t('healthPath')}: {active.health_path}
                </Badge>
              </>
            )}
          </div>
          {active.build_type === 'static' ? (
            <p className="text-muted-foreground">{t('static')}</p>
          ) : null}
          {active.env_hints.length > 0 ? (
            <div className="space-y-1">
              <p className="font-medium">{t('envHints')}</p>
              <ul className="space-y-1 text-muted-foreground">
                {active.env_hints.map((e) => (
                  <li key={e.key}>
                    <code className="font-mono text-xs text-foreground">
                      {e.key}
                    </code>
                    {e.required ? ` (${t('required')})` : ''}: {e.description}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {active.volumes.length > 0 ? (
            <div className="space-y-1">
              <p className="font-medium">{t('volumes')}</p>
              <ul className="space-y-1 text-muted-foreground">
                {active.volumes.map((v) => (
                  <li key={v.path}>
                    <code className="font-mono text-xs text-foreground">
                      {v.path}
                    </code>
                    : {v.purpose}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {active.dockerfile_template ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                void copyDockerfile(active)
              }}
            >
              <CopyGlassIcon className="size-4.5" />
              {t('dockerfile')}
            </Button>
          ) : null}
        </div>
      ) : null}

      {active && presetDiffers(active, port, healthPath) ? (
        <Alert>
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
          <AlertTitle>{t('mismatchTitle')}</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>
              {t('mismatchBody', {
                port: port || '-',
                health: healthPath || '-',
                name: active.name,
                presetPort: active.port,
                presetHealth: active.health_path,
              })}
            </p>
            <Button
              type="button"
              size="sm"
              disabled={disabled}
              onClick={() => {
                onApply(active)
                toast.add({ title: t('applied'), type: 'success' })
              }}
            >
              {t('applyFix')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
