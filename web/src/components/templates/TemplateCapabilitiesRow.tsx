import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import type { TemplateCapability } from '../../queries/serviceTemplates'

const TIER_VARIANT = {
  review: 'warning',
  elevated: 'destructive',
  privileged: 'destructive',
} as const satisfies Record<TemplateCapability['tier'], string>

/** Declared capability grants, each with its tier and a short plain explanation. */
export function TemplateCapabilitiesRow({
  capabilities,
}: {
  capabilities?: TemplateCapability[]
}) {
  const { t } = useTranslation('templates')
  if (!capabilities?.length) return null
  return (
    <section aria-label={t('capabilities.title')} className="space-y-1.5">
      <h3 className="text-xs font-medium text-muted-foreground">
        {t('capabilities.title')}
      </h3>
      <ul className="space-y-1.5 text-sm">
        {capabilities.map((c) => (
          <li
            key={`${c.service}-${c.class}`}
            className="flex flex-wrap items-baseline gap-x-2 gap-y-1"
          >
            <Badge variant={TIER_VARIANT[c.tier]}>
              {t(`capabilities.tier.${c.tier}`)}
            </Badge>
            <span className="font-mono text-xs">
              {c.service}
              {c.caps?.length ? `: ${c.caps.join(', ')}` : ''}
            </span>
            <span className="min-w-0 text-muted-foreground">
              {t(`capabilities.class.${c.class}`, { defaultValue: c.class })}
              {c.reason ? ` ${c.reason}` : ''}
            </span>
          </li>
        ))}
      </ul>
    </section>
  )
}
