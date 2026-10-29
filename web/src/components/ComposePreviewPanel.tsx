import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  InfoGlassIcon,
  ShieldAlertGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { useComposePreview } from '../queries/composeStack'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import type { ComposePreviewService } from '../types/compose'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'

const PREVIEW_DEBOUNCE_MS = 600

// Server-side parse of the pasted file, so the wizard shows exactly what
// the deploy path will accept: services, volumes, env kinds, every error.
export function ComposePreviewPanel({ composeYaml }: { composeYaml: string }) {
  const { t } = useTranslation('compose')
  const body = useDebouncedValue(composeYaml, PREVIEW_DEBOUNCE_MS)
  const preview = useComposePreview(body, [])

  if (!body.trim()) return null
  if (preview.isPending) {
    return (
      <p className="text-xs text-muted-foreground" aria-live="polite">
        {t('preview.checking')}
      </p>
    )
  }
  if (preview.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
        <AlertDescription>
          {t('preview.loadFailed', { message: preview.error.message })}
        </AlertDescription>
      </Alert>
    )
  }

  const data = preview.data
  return (
    <section
      className="space-y-3 rounded-lg border border-border p-3"
      aria-label={t('preview.title')}
    >
      <div className="flex items-center gap-2 text-sm font-medium text-foreground">
        {data.valid ? (
          <CircleCheckGlassIcon
            className="size-4.5 text-green-600 dark:text-green-400"
            aria-hidden="true"
          />
        ) : (
          <TriangleAlertGlassIcon
            className="size-4.5 text-destructive"
            aria-hidden="true"
          />
        )}
        {data.valid
          ? t('preview.valid', { count: data.services.length })
          : t('preview.invalid')}
      </div>
      {data.errors?.length ? (
        <ul className="list-disc space-y-1 pl-5 text-xs text-destructive">
          {data.errors.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      ) : null}
      {data.needs_root ? (
        <Alert>
          <ShieldAlertGlassIcon className="text-tone-warning" size="1.2em" />
          <AlertDescription>{t('preview.needsRoot')}</AlertDescription>
        </Alert>
      ) : null}
      {[
        ...(data.warnings ?? []),
        ...(data.notices ?? []).map((n) => n.message),
      ].map((msg) => (
        <p key={msg} className="flex gap-1.5 text-xs text-muted-foreground">
          <InfoGlassIcon
            className="mt-0.5 size-4 shrink-0 text-tone-info"
            aria-hidden="true"
          />
          {msg}
        </p>
      ))}
      {data.skipped_services?.length ? (
        <p className="text-xs text-muted-foreground">
          {t('preview.skipped', { services: data.skipped_services.join(', ') })}
        </p>
      ) : null}
      {data.profiles?.length ? (
        <p className="text-xs text-muted-foreground">
          {t('preview.profilesAvailable', {
            profiles: data.profiles.join(', '),
          })}
        </p>
      ) : null}
      <ul className="space-y-2">
        {data.services.map((svc) => (
          <PreviewServiceRow key={svc.key} svc={svc} />
        ))}
      </ul>
    </section>
  )
}

function PreviewServiceRow({ svc }: { svc: ComposePreviewService }) {
  const { t } = useTranslation('compose')
  return (
    <li className="space-y-1 rounded-md bg-muted/40 p-2 text-xs">
      <div className="flex flex-wrap items-center gap-1.5">
        <span className="font-medium text-foreground">{svc.key}</span>
        <span className="font-mono text-muted-foreground">
          {svc.build ? t('preview.build') : svc.image}
        </span>
        {svc.port ? (
          <Badge variant="outline">
            {t('preview.port', { port: svc.port })}
          </Badge>
        ) : null}
        {svc.domain ? (
          <Badge variant="info">
            {t('preview.domain', { domain: svc.domain })}
          </Badge>
        ) : null}
        {svc.depends_on?.length ? (
          <Badge variant="muted">
            {t('preview.dependsOn', { services: svc.depends_on.join(', ') })}
          </Badge>
        ) : null}
      </div>
      {svc.volumes?.length ? (
        <p className="text-muted-foreground">
          {t('preview.volumes')}: {svc.volumes.join(', ')}
        </p>
      ) : null}
      {svc.env?.length ? (
        <div className="flex flex-wrap items-center gap-1 text-muted-foreground">
          <span>{t('preview.env')}:</span>
          {svc.env.map((e) => (
            <Badge
              key={e.key}
              variant={
                e.required ? 'warning' : e.generated ? 'success' : 'muted'
              }
            >
              {e.key}
              {e.generated ? ` (${t('preview.generated')})` : ''}
              {e.required ? ` (${t('preview.required')})` : ''}
              {e.secret && !e.generated ? ` (${t('preview.secret')})` : ''}
            </Badge>
          ))}
        </div>
      ) : null}
      {svc.healthcheck ? (
        <p className="truncate font-mono text-muted-foreground">
          {t('preview.healthcheck', { command: svc.healthcheck })}
        </p>
      ) : null}
    </li>
  )
}
