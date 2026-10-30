import { useTranslation } from 'react-i18next'
import { useRouterState } from '@tanstack/react-router'
import { SectionTabs, type SectionTabItem } from './SectionTabs'
import {
  PLATFORM_OVERVIEW_PATH,
  type PlatformTabTo,
} from '@/lib/platformSettingsIndex'
import { matchesPrefix } from './navModel'

/** Deploys, Timeline and Builds share one sidebar entry; each stays its own route. */
export function DeploymentsSectionTabs() {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <SectionTabs
      ariaLabel={t('shell.sectionTabs.deployments.label')}
      items={[
        {
          to: '/deployments',
          label: t('shell.sectionTabs.deployments.deploys'),
          active: matchesPrefix(pathname, '/deployments'),
        },
        {
          to: '/deploy-timeline',
          label: t('shell.sectionTabs.deployments.timeline'),
          active: matchesPrefix(pathname, '/deploy-timeline'),
        },
        {
          to: '/builds',
          label: t('shell.sectionTabs.deployments.builds'),
          active: matchesPrefix(pathname, '/builds'),
        },
      ]}
    />
  )
}

/** Usage and the cost explorer share one sidebar entry. */
export function CostSectionTabs() {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  return (
    <SectionTabs
      ariaLabel={t('shell.sectionTabs.cost.label')}
      items={[
        {
          to: '/usage',
          label: t('shell.sectionTabs.cost.usage'),
          active: matchesPrefix(pathname, '/usage'),
        },
        {
          to: '/costs',
          label: t('shell.sectionTabs.cost.costs'),
          active: matchesPrefix(pathname, '/costs'),
        },
      ]}
    />
  )
}

/** Platform settings pages share one sidebar entry; the overview is exact so it is not lit on its children. */
export function PlatformSectionTabs() {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const tab = (to: PlatformTabTo, label: string): SectionTabItem => ({
    to,
    label,
    active:
      to === PLATFORM_OVERVIEW_PATH
        ? pathname.replace(/\/$/, '') === PLATFORM_OVERVIEW_PATH
        : matchesPrefix(pathname, to),
  })
  return (
    <SectionTabs
      ariaLabel={t('shell.sectionTabs.platform.label')}
      items={[
        tab('/settings/platform', t('shell.sectionTabs.platform.overview')),
        tab('/settings/platform/https', t('shell.sectionTabs.platform.https')),
        tab(
          '/settings/platform/ingress',
          t('shell.sectionTabs.platform.ingress'),
        ),
        tab(
          '/settings/platform/addresses',
          t('shell.sectionTabs.platform.addresses'),
        ),
        tab(
          '/settings/platform/dns-providers',
          t('shell.sectionTabs.platform.dnsProviders'),
        ),
      ]}
    />
  )
}
