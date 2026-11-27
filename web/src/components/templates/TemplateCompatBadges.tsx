import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { useTemplateCompat } from '../../queries/templateCompat'
import type { ServiceTemplateListItem } from '../../queries/serviceTemplates'

/** Needs and arm64 gaps for one card: local install failures plus the registry arch facts. */
export function TemplateCompatBadges({
  template,
}: {
  template: ServiceTemplateListItem
}) {
  const { t } = useTranslation('templates')
  const { data } = useTemplateCompat()
  const rows = (data ?? []).filter((r) => r.template_id === template.id)
  const noArm = template.arch?.arm64 === 'absent'
  if (rows.length === 0 && !noArm) return null
  return (
    <>
      {rows.map((r) => (
        <Badge
          key={`${r.class}-${r.node_arch}`}
          variant="warning"
          className="text-xs"
          title={r.fix}
        >
          {t('compat.needs', {
            what: t(`compat.class.${r.class}`, { defaultValue: r.class }),
          })}
        </Badge>
      ))}
      {noArm ? (
        <Badge
          variant="muted"
          className="text-xs"
          title={t('compat.noArm64Hint')}
        >
          {t('compat.noArm64')}
        </Badge>
      ) : null}
    </>
  )
}
