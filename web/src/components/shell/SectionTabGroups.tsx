import { useTranslation } from 'react-i18next'
import { useRouterState } from '@tanstack/react-router'
import { SectionTabs } from './SectionTabs'
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
