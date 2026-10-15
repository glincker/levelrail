import { useTranslation } from 'react-i18next'
import { groupEnvNames } from '../../lib/templateEnv'
import type { ComposePreviewService } from '../../types/compose'

function NameList({ label, names }: { label: string; names: string[] }) {
  if (names.length === 0) return null
  return (
    <div className="space-y-1.5">
      <h3 className="text-xs font-medium text-muted-foreground">{label}</h3>
      <ul className="flex flex-wrap gap-1.5">
        {names.map((n) => (
          <li
            key={n}
            className="rounded-md border border-border bg-muted/40 px-2 py-0.5 font-mono text-xs"
          >
            {n}
          </li>
        ))}
      </ul>
    </div>
  )
}

export function TemplateEnvNames({
  services,
}: {
  services: ComposePreviewService[]
}) {
  const { t } = useTranslation('templates')
  const g = groupEnvNames(services)
  if (g.required.length + g.generated.length + g.optional.length === 0) {
    return null
  }
  return (
    <section aria-labelledby="template-env" className="space-y-3">
      <div>
        <h2 id="template-env" className="text-base font-medium">
          {t('detail.env.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('detail.env.description')}
        </p>
      </div>
      <NameList label={t('detail.env.required')} names={g.required} />
      <NameList label={t('detail.env.generated')} names={g.generated} />
      <NameList label={t('detail.env.optional')} names={g.optional} />
    </section>
  )
}
