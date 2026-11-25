import { createFileRoute, redirect } from '@tanstack/react-router'
import { SettingsShell } from '../../../components/databases/home/SettingsShell'
import {
  MOVED_SECTIONS,
  SETTINGS_SECTIONS,
  type MovedSection,
  type SettingsSection,
} from '../../../components/databases/home/settingsSections'

export const Route = createFileRoute('/databases/$name/settings')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { section: SettingsSection; moved?: MovedSection } => ({
    section: SETTINGS_SECTIONS.find((s) => s === search.section) ?? 'general',
    moved: MOVED_SECTIONS.find((s) => s === search.section),
  }),
  beforeLoad: ({ params, search }) => {
    if (search.moved === 'networking') {
      redirect({
        to: '/databases/$name/networking',
        params,
        throw: true,
      })
    }
    if (search.moved === 'credentials') {
      redirect({
        to: '/databases/$name/credentials',
        params,
        throw: true,
      })
    }
  },
  component: SettingsSection,
})

function SettingsSection() {
  const { name } = Route.useParams()
  const { section } = Route.useSearch()
  return <SettingsShell name={name} section={section} />
}
