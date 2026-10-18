import { useTranslation } from 'react-i18next'
import type { Plugin } from '../../queries/plugins'

export function PluginPanels({ plugins }: Readonly<{ plugins: Plugin[] }>) {
  const { t } = useTranslation('plugins')
  const shown = plugins.filter(
    (p) => p.state === 'enabled' && p.panels.length > 0,
  )
  if (shown.length === 0) return null
  return (
    <section className="space-y-3" aria-label={t('panels.title')}>
      <h2 className="text-base font-medium">{t('panels.title')}</h2>
      <div className="grid gap-3 md:grid-cols-2">
        {shown.flatMap((p) =>
          p.panels.map((panel) => (
            <article
              key={`${p.name}/${panel.id}`}
              className="rounded-lg border bg-card p-4 text-card-foreground"
            >
              <h3 className="font-medium">{panel.title}</h3>
              <p className="text-xs text-muted-foreground">
                {t('panels.from', { name: p.name, publisher: p.publisher })}
              </p>
              {panel.description ? (
                <p className="mt-2 text-sm">{panel.description}</p>
              ) : null}
              <dl className="mt-3 space-y-1 text-sm">
                {panel.rows.map((row) => (
                  <div key={row.label} className="flex justify-between gap-4">
                    <dt className="text-muted-foreground">{row.label}</dt>
                    <dd className="text-right break-all">{row.value}</dd>
                  </div>
                ))}
              </dl>
            </article>
          )),
        )}
      </div>
    </section>
  )
}
