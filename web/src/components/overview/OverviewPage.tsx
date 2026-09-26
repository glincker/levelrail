import { useEffect, useState } from 'react'
import {
  ArrowClockwiseIcon,
  ArrowSquareOutIcon,
  CopyIcon,
  RocketLaunchIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useGitSource } from '../../queries/gitSources'
import { registerPageActions } from '../../lib/pageActions'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import type { DeployAttempt } from '../../types/deployAttempt'
import { ActivityTimeline } from './ActivityTimeline'
import { DeploySheet, type DeployTab } from './DeploySheet'
import { DetailsSection } from './DetailsSection'
import { LogsTail } from './LogsTail'
import { OverviewHero } from './OverviewHero'
import { OverviewSuggestions } from './OverviewSuggestions'
import { ResourceTiles } from './ResourceTiles'
import { useResourceReading } from './useResourceReading'
import { SetupRing } from './SetupRing'
import { TrafficTiles } from './TrafficTiles'
import { useAppUrl } from './useAppUrl'
import { useOverviewActions } from './useOverviewActions'
import { useOverviewHotkeys } from './useOverviewHotkeys'

export function OverviewPage({
  app,
  conditions,
  attempts,
}: {
  app: AppDetail
  conditions: ReconcileCondition[]
  attempts: DeployAttempt[]
}) {
  const [deployTab, setDeployTab] = useState<DeployTab | null>(null)
  const url = useAppUrl(app)
  const latest = attempts[0]
  const reading = useResourceReading(app)
  const git = useGitSource(app.name)
  const actions = useOverviewActions(app, url)
  const { openApp, copyUrl, restart, redeploy } = actions

  useEffect(
    () =>
      registerPageActions([
        {
          key: 'page-open-app',
          label: `Open ${app.name}`,
          icon: <ArrowSquareOutIcon />,
          run: openApp,
          hint: ['O'],
        },
        {
          key: 'page-deploy',
          label: `Deploy ${app.name}...`,
          icon: <RocketLaunchIcon />,
          run: () => setDeployTab('existing-image'),
          hint: ['Shift', 'D'],
        },
        {
          key: 'page-restart',
          label: `Restart ${app.name}`,
          icon: <ArrowClockwiseIcon />,
          run: restart,
        },
        {
          key: 'page-redeploy',
          label: `Redeploy ${app.name}`,
          icon: <RocketLaunchIcon />,
          run: redeploy,
        },
        {
          key: 'page-copy-url',
          label: `Copy URL of ${app.name}`,
          icon: <CopyIcon />,
          run: copyUrl,
          hint: ['C'],
        },
      ]),
    [app.name, openApp, copyUrl, restart, redeploy],
  )
  useOverviewHotkeys({
    onOpenApp: openApp,
    onCopyUrl: copyUrl,
    onDeploy: () => setDeployTab('existing-image'),
  })

  return (
    <div className="space-y-6">
      <OverviewHero
        app={app}
        conditions={conditions}
        latestAttempt={latest}
        url={url}
        onDeploy={setDeployTab}
      />
      <SetupRing app={app} hasGitSource={Boolean(git.data)} />
      <OverviewSuggestions app={app} reading={reading} latest={latest} />
      <section
        aria-label="Live traffic and resources"
        className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5"
      >
        <TrafficTiles appName={app.name} url={url} />
        <ResourceTiles reading={reading} />
      </section>
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <ActivityTimeline
          app={app}
          attempts={attempts}
          conditions={conditions}
        />
        <LogsTail appName={app.name} />
      </div>
      <DetailsSection app={app} conditions={conditions} />
      <DeploySheet
        appName={app.name}
        tab={deployTab}
        onClose={() => setDeployTab(null)}
      />
    </div>
  )
}
