import { useTranslation } from 'react-i18next'
import { SETTINGS_GROUP_IDS } from './groups'

// Plain anchor links: keyboard and screen reader friendly with no script,
// and the browser's own jump honours prefers-reduced-motion.
export function SettingsSectionNav() {
  const { t } = useTranslation('common')
  return (
    <nav
      aria-label={t('deploySettings.nav.label')}
      className="sticky top-0 z-10 -mx-1 border-b border-border bg-background/95 px-1 py-2 backdrop-blur supports-[backdrop-filter]:bg-background/80"
    >
      <ul className="flex flex-wrap gap-1">
        {SETTINGS_GROUP_IDS.map((id) => (
          <li key={id}>
            <a
              href={`#${id}`}
              className="inline-flex rounded-md px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none motion-reduce:transition-none"
            >
              {t(`deploySettings.groups.${id}.title`)}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  )
}
