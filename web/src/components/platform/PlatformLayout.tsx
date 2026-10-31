import { Outlet, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CloudCheckGlassIcon,
  GlobeGlassIcon,
  LockGlassIcon,
  PlugGlassIcon,
  SettingsGlassIcon,
} from '@thesvg/ui-react'
import { PLATFORM_PAGE_PATH } from '../../lib/platformSettingsIndex'
import { PlatformSectionTabs } from '../shell/SectionTabGroups'
import { PlatformPageShell } from './PlatformPageShell'

// One shell for every platform page: the header and tabs stay mounted while
// the tab body swaps, so switching tabs does not remount the page chrome.
export function PlatformLayout() {
  const { t } = useTranslation('platformSettings')
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const page = pathname.startsWith(PLATFORM_PAGE_PATH.https)
    ? {
        title: t('pages.https.title'),
        description: t('pages.https.description'),
        icon: <LockGlassIcon size="1.4em" />,
      }
    : pathname.startsWith(PLATFORM_PAGE_PATH.ingress)
      ? {
          title: t('pages.ingress.title'),
          description: t('pages.ingress.description'),
          icon: <PlugGlassIcon size="1.4em" />,
        }
      : pathname.startsWith(PLATFORM_PAGE_PATH.addresses)
        ? {
            title: t('pages.addresses.title'),
            description: t('pages.addresses.description'),
            icon: <GlobeGlassIcon size="1.4em" />,
          }
        : pathname.startsWith(PLATFORM_PAGE_PATH['dns-providers'])
          ? {
              title: t('pages.dnsProviders.title'),
              description: t('pages.dnsProviders.description'),
              icon: <CloudCheckGlassIcon size="1.4em" />,
            }
          : {
              title: t('overview.title'),
              description: t('overview.description'),
              icon: <SettingsGlassIcon size="1.4em" />,
            }
  return (
    <PlatformPageShell
      title={page.title}
      description={page.description}
      icon={page.icon}
      tabs={<PlatformSectionTabs />}
    >
      <Outlet />
    </PlatformPageShell>
  )
}
