import { useTranslation } from 'react-i18next'
import type { SettingsItemId } from '../../lib/settingsNav'
import { PageHeader } from '../shell/PageHeader'

// Title row for a settings page's loading state, labelled from the same
// nav catalogue the hub and sidebar use.
export function SettingsPageTitle({ id }: { id: SettingsItemId }) {
  const { t } = useTranslation('settings')
  return <PageHeader title={t(`nav.items.${id}.title`)} />
}
