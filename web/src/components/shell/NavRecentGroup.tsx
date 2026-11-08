import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon, LayersGlassIcon } from '@thesvg/ui-react'
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { parseRecent, type RecentEntry } from '@/lib/navPrefs'
import { CollapsibleSection, NAV_ROW_CLASS } from './CollapsibleSection'

function RecentItem({
  entry,
  active,
}: {
  entry: RecentEntry
  active: boolean
}) {
  const { t } = useTranslation('sidebarCustomize')
  const isApp = entry.kind === 'app'
  const Icon = isApp ? LayersGlassIcon : DatabaseGlassIcon
  const link = isApp ? (
    <Link to="/apps/$name" params={{ name: entry.name }} />
  ) : (
    <Link to="/databases/$name" params={{ name: entry.name }} />
  )
  return (
    <SidebarMenuItem>
      <SidebarMenuButton
        render={link}
        isActive={active}
        className={NAV_ROW_CLASS}
        title={entry.name}
      >
        <Icon
          size="1.4em"
          aria-label={t(isApp ? 'recent.app' : 'recent.database')}
          role="img"
        />
        <span>{entry.name}</span>
      </SidebarMenuButton>
    </SidebarMenuItem>
  )
}

// Loaded on demand: only users with visited apps or databases ever render it.
export function NavRecentGroup({
  raw,
  pathname,
  open,
  onOpenChange,
}: {
  raw: string
  pathname: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('sidebarCustomize')
  const recent = parseRecent(raw)
  if (recent.length === 0) return null
  return (
    <CollapsibleSection
      id="global-recent"
      open={open}
      onOpenChange={onOpenChange}
      header={t('recent.group')}
    >
      <SidebarMenu>
        {recent.map((entry) => (
          <RecentItem
            key={`${entry.kind}:${entry.name}`}
            entry={entry}
            active={pathname.startsWith(
              `/${entry.kind === 'app' ? 'apps' : 'databases'}/${encodeURIComponent(entry.name)}`,
            )}
          />
        ))}
      </SidebarMenu>
    </CollapsibleSection>
  )
}
