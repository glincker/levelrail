import { useTranslation } from 'react-i18next'
import type { AppListEntry } from '../../types/appDetail'
import { CARD_GRID_CLASS } from './AppCardSkeleton'
import { AppPreviewCard } from './AppPreviewCard'

export default function AppsCardGrid({
  apps,
  staticNames,
  selected,
  onSelect,
}: Readonly<{
  apps: readonly AppListEntry[]
  staticNames: ReadonlySet<string>
  selected: readonly string[]
  onSelect: (name: string, checked: boolean) => void
}>) {
  const { t } = useTranslation('appRows')
  if (apps.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
        {t('card.empty')}
      </p>
    )
  }
  return (
    <ul aria-label={t('card.listLabel')} className={CARD_GRID_CLASS}>
      {apps.map((app) => (
        <AppPreviewCard
          key={app.name}
          app={app}
          isStatic={staticNames.has(app.name)}
          selected={selected.includes(app.name)}
          onSelect={onSelect}
        />
      ))}
    </ul>
  )
}
