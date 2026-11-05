import { useSettingsNav } from '../../hooks/useSettingsNav'
import type { SettingsItemId } from '../../lib/settingsNav'
import { PageHeader } from '../shell/PageHeader'

// Title row for a settings page's loading state, labelled from the same
// nav catalogue the hub and sidebar use.
export function SettingsPageTitle({ id }: { id: SettingsItemId }) {
  const sections = useSettingsNav()
  const title =
    sections.flatMap((s) => s.items).find((item) => item.id === id)?.title ?? ''
  return <PageHeader title={title} />
}
