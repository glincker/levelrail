import { useTranslation } from 'react-i18next'
import { KeyGlassIcon, PlugGlassIcon, ServerGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { envKind, imagePin } from '../../lib/templateTrust'
import type { EnvKind } from '../../lib/templateTrust'
import type { ComposePreviewService } from '../../types/compose'

const ENV_STYLE: Record<EnvKind, string> = {
  required: 'border-primary/50 text-foreground',
  generated: 'border-border bg-muted/40 text-muted-foreground',
  optional: 'border-dashed border-border text-muted-foreground',
}

/** Pre-deploy preview: each compose service with image, ports, volumes and env names (never values). */
export function TemplateServices({
  services,
}: {
  services: ComposePreviewService[]
}) {
  const { t } = useTranslation('templates')
  const main = services.find((s) => s.port) ?? services[0]
  return (
    <section aria-labelledby="template-deployed" className="space-y-3">
      <div>
        <h2 id="template-deployed" className="text-base font-semibold">
          {t('install.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('install.description', { count: services.length })}
        </p>
      </div>
      <ul className="space-y-2">
        {services.map((s) => {
          const deps = [
            ...(s.depends_on ?? []),
            ...(s.depends_on_healthy ?? []),
          ]
          const volumes = [...(s.volumes ?? []), ...(s.bind_mounts ?? [])]
          const env = s.env ?? []
          return (
            <li
              key={s.key}
              className="rounded-lg border border-border bg-card p-3"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-foreground">
                  {s.key}
                </span>
                <Badge variant="muted">
                  {s === main
                    ? t('detail.deployed.roleMain')
                    : t('detail.deployed.roleSupport')}
                </Badge>
                {s.port ? (
                  <Badge variant="outline">
                    <PlugGlassIcon className="size-3.5" aria-hidden="true" />
                    {t('detail.deployed.port', { port: s.port })}
                  </Badge>
                ) : null}
                {s.image ? (
                  <Badge variant="outline">
                    {t(`install.pin.${imagePin(s.image)}`)}
                  </Badge>
                ) : null}
              </div>
              {s.image ? (
                <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                  {s.image}
                </p>
              ) : null}
              {volumes.length > 0 ? (
                <p className="mt-1 flex items-start gap-1.5 text-xs text-muted-foreground">
                  <ServerGlassIcon
                    className="mt-0.5 size-4 shrink-0"
                    aria-label={t('install.volumes')}
                  />
                  <span className="min-w-0 font-mono break-all">
                    {volumes.join(', ')}
                  </span>
                </p>
              ) : null}
              {env.length > 0 ? (
                <ul
                  className="mt-2 flex flex-wrap gap-1.5"
                  aria-label={t('install.env')}
                >
                  {env.map((e) => {
                    const kind = envKind(e)
                    return (
                      <li
                        key={e.key}
                        className={`inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-xs ${ENV_STYLE[kind]}`}
                      >
                        {e.secret || kind !== 'optional' ? (
                          <KeyGlassIcon
                            className="size-3.5"
                            aria-hidden="true"
                          />
                        ) : null}
                        <span className="font-mono">{e.key}</span>
                        <span className="text-muted-foreground">
                          {t(`install.envKind.${kind}`)}
                        </span>
                      </li>
                    )
                  })}
                </ul>
              ) : null}
              {deps.length > 0 ? (
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('detail.deployed.dependsOn', { names: deps.join(', ') })}
                </p>
              ) : null}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
