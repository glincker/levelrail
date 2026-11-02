import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PaletteGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { AppearanceSettings } from '@/components/settings/AppearanceSettings'

export const Route = createFileRoute('/settings/appearance')({
  component: AppearanceSettingsPage,
})

function AppearanceSettingsPage() {
  const { t } = useTranslation('settings')
  return (
    <PageTemplate
      icon={<PaletteGlassIcon size="1.4em" />}
      title={t('appearancePage.title')}
      description={t('appearancePage.description')}
    >
      <AppearanceSettings />
    </PageTemplate>
  )
}
