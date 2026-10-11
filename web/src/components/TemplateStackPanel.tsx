import { useTranslation } from 'react-i18next'
import type { TemplateManifest, TemplateStackService } from '../types/compose'

// "What this installs" for a manifest template: one row per service with
// its image, container ports and named volumes, plus where the template
// was converted from when it has a source block.
export function TemplateStackPanel({
  stack,
  source,
}: {
  stack: TemplateStackService[]
  source?: TemplateManifest['source']
}) {
  const { t } = useTranslation('compose')
  if (stack.length === 0) return null
  return (
    <section
      aria-labelledby="template-stack-title"
      className="space-y-3 rounded-lg border border-border bg-card p-4"
    >
      <div>
        <h2 id="template-stack-title" className="text-sm font-semibold">
          {t('catalog.stackTitle')}
        </h2>
        <p className="text-xs text-muted-foreground">
          {t('catalog.stackDescription')}
        </p>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-left text-sm">
          <thead className="text-xs text-muted-foreground">
            <tr>
              <th className="py-1 pr-3 font-medium">
                {t('catalog.stackService')}
              </th>
              <th className="py-1 pr-3 font-medium">
                {t('catalog.stackImage')}
              </th>
              <th className="py-1 pr-3 font-medium">
                {t('catalog.stackPorts')}
              </th>
              <th className="py-1 font-medium">{t('catalog.stackVolumes')}</th>
            </tr>
          </thead>
          <tbody>
            {stack.map((service) => (
              <tr key={service.name} className="border-t border-border">
                <td className="py-1.5 pr-3 font-medium">{service.name}</td>
                <td className="py-1.5 pr-3 font-mono text-xs break-all">
                  {service.image}
                </td>
                <td className="py-1.5 pr-3 tabular-nums">
                  {service.ports?.length
                    ? service.ports.join(', ')
                    : t('catalog.none')}
                </td>
                <td className="py-1.5 font-mono text-xs break-all">
                  {service.volumes?.length
                    ? service.volumes.join(', ')
                    : t('catalog.none')}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {source ? (
        <p className="text-xs text-muted-foreground">
          {t('catalog.sourceLine', {
            name: source.name,
            path: source.path,
            license: source.license,
          })}
        </p>
      ) : null}
    </section>
  )
}
