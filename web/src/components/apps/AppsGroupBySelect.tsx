import { useTranslation } from 'react-i18next'
import { GROUP_BY_OPTIONS, type GroupBy } from '../../lib/appsGroup'

export function AppsGroupBySelect({
  value,
  onChange,
}: {
  value: GroupBy
  onChange: (next: GroupBy) => void
}) {
  const { t } = useTranslation('appRows')
  return (
    <select
      aria-label={t('group.label')}
      value={value}
      onChange={(e) => {
        onChange(e.target.value as GroupBy)
      }}
      className="h-8 rounded-lg border border-border bg-background px-2 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
    >
      {GROUP_BY_OPTIONS.map((o) => (
        <option key={o} value={o}>
          {o === 'none'
            ? t('group.none')
            : `${t('group.label')}: ${t(`group.${o}`)}`}
        </option>
      ))}
    </select>
  )
}
