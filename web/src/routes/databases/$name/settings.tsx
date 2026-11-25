import { createFileRoute } from '@tanstack/react-router'
import { SettingsShell } from '../../../components/databases/home/SettingsShell'
import {
  SETTINGS_SECTIONS,
  type SettingsSection,
} from '../../../components/databases/home/settingsSections'

export const Route = createFileRoute('/databases/$name/settings')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { section: SettingsSection } => ({
    section: SETTINGS_SECTIONS.find((s) => s === search.section) ?? 'general',
  }),
  component: SettingsSection,
})

function SettingsSection() {
  const { name } = Route.useParams()
  const { section } = Route.useSearch()
  return <SettingsShell name={name} section={section} />
}
