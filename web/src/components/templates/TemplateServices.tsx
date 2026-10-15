import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { ComposePreviewService } from '../../types/compose'

/** What gets deployed: each compose service with its image, port and role. */
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
        <h2 id="template-deployed" className="text-base font-medium">
          {t('detail.deployed.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('detail.deployed.description')}
        </p>
      </div>
      <ul className="space-y-2">
        {services.map((s) => {
          const deps = [
            ...(s.depends_on ?? []),
            ...(s.depends_on_healthy ?? []),
          ]
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
                    {t('detail.deployed.port', { port: s.port })}
                  </Badge>
                ) : null}
              </div>
              {s.image ? (
                <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
                  {s.image}
                </p>
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
