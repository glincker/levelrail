import {
  Fragment,
  Suspense,
  lazy,
  useEffect,
  useState,
  type ReactNode,
} from 'react'
import {
  CopyGlassIcon,
  ExternalLinkGlassIcon,
  RocketGlassIcon,
  RotateCwGlassIcon,
} from '@thesvg/ui-react'
import { useAppStatus } from '../../queries/appStatus'
import { useGitSource } from '../../queries/gitSources'
import { ApiError } from '../../lib/apiError'
import { useIsStaticSite } from '../../queries/staticSites'
import { DropDeployCard } from '../dropdeploy/DropDeployCard'
import { registerPageActions } from '../../lib/pageActions'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import type { DeployAttempt } from '../../types/deployAttempt'
import { DiagnosisCard } from './DiagnosisCard'
import { useDiagnosisShown } from './useDiagnosisShown'
import { RecommendedNextCard } from './RecommendedNextCard'
import { AppHealthScorePanel } from '../AppHealthScorePanel'
import { ActivityTimeline } from './ActivityTimeline'
import { DeploySheet, type DeployTab } from './DeploySheet'
import { DetailsSection } from './DetailsSection'
import { UploadSourceCard } from './UploadSourceCard'
import { latestUpload } from './uploadSource'
import { LogsTail } from './LogsTail'
import { OverviewHero } from './OverviewHero'
import { OverviewQuickLinks } from './OverviewQuickLinks'
import { OverviewSuggestions } from './OverviewSuggestions'
import { useOverviewSuggestions } from './useOverviewSuggestions'
import { RecommendedSetup } from './RecommendedSetup'
import { useSetupDismissed } from './useSetupDismissed'
import { computeOverviewLayout, type OverviewSection } from './overviewLayout'
import { computeSetup } from './suggestions'
import { AppAccessCard } from '../AppAccessCard'
import { AppAccessDetails } from '../templates/AppAccessDetails'
import { useAppUrls } from '../../queries/appUrls'
import { pickOpenUrl } from '../appAccessRows'
import { useAppTraffic } from '../../queries/appTraffic'
import { ResourceTiles } from './ResourceTiles'
import { useResourceReading } from './useResourceReading'
import { isImageDeploy } from './suggestions'
import { TrafficTiles } from './TrafficTiles'
import { VisitorsCard } from './VisitorsCard'
import { StaticStatStrip } from './StaticStatStrip'
import { RecentDeploys } from './RecentDeploys'
import { AppDeploys } from './AppDeploys'
import { AppRecentPreviews } from './AppRecentPreviews'
import { AppSummaryStrip } from './AppSummaryStrip'
import { AppWiring } from './AppWiring'
import type { VisitorsWindow } from '../../queries/appVisitors'
import { useAppUrl } from './useAppUrl'
import { useOverviewActions } from './useOverviewActions'
import { useOverviewHotkeys } from './useOverviewHotkeys'

// Lazy so the preview (and its iframe code) stays out of the route chunk.
const OverviewPreview = lazy(() => import('./OverviewPreview'))

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
  const [range, setRange] = useState<VisitorsWindow>('24h')
  const baseUrl = useAppUrl(app)
  const { data: urls } = useAppUrls(app.name)
  const openUrl = baseUrl ?? pickOpenUrl(urls)
  const url = openUrl
  const latest = attempts[0]
  const reading = useResourceReading(app)
  const git = useGitSource(app.name)
  const isStatic = useIsStaticSite(app.name)
  const uploadAttempt = git.data ? undefined : latestUpload(attempts)
  const appStatus = useAppStatus(app.name).data
  const rawSuggestions = useOverviewSuggestions(
    app,
    reading,
    latest,
    conditions,
  )
  const diagnosisShown = useDiagnosisShown(app.name)
  // The Diagnosis card already states the failure, so its twin suggestions are dropped.
  const suggestions = diagnosisShown
    ? {
        ...rawSuggestions,
        attention: rawSuggestions.attention.filter(
          (d) => d.id !== 'failed_deploy' && d.id !== 'unhealthy',
        ),
      }
    : rawSuggestions
  const setup = useSetupDismissed(app.name)
  const { data: traffic } = useAppTraffic(app.name)
  const setupState = computeSetup({
    app,
    hasGitSource: Boolean(git.data),
    imageDeployed: isImageDeploy(latest?.source),
  })
  const layout = computeOverviewLayout({
    attentionCount: suggestions.attention.length,
    diagnosisShown,
    hasTraffic: Boolean(traffic?.hasTraffic),
    trafficKnown: traffic !== undefined,
    suspended: app.suspended,
    isStatic,
    setupRemaining:
      setupState.items.length - setupState.done + suggestions.setup.length,
    setupDismissed: setup.dismissed,
  })
  const actions = useOverviewActions(app, url)
  const { openApp, copyUrl, restart, redeploy } = actions

  useEffect(
    () =>
      registerPageActions([
        {
          key: 'page-open-app',
          label: `Open ${app.name}`,
          icon: <ExternalLinkGlassIcon size="1.4em" />,
          run: openApp,
          hint: ['O'],
        },
        {
          key: 'page-deploy',
          label: `Deploy ${app.name}...`,
          icon: <RocketGlassIcon size="1.4em" />,
          run: () => setDeployTab('existing-image'),
          hint: ['Shift', 'D'],
        },
        {
          key: 'page-restart',
          label: `Restart ${app.name}`,
          icon: <RotateCwGlassIcon size="1.4em" />,
          run: restart,
        },
        {
          key: 'page-redeploy',
          label: `Redeploy ${app.name}`,
          icon: <RocketGlassIcon size="1.4em" />,
          run: redeploy,
        },
        {
          key: 'page-copy-url',
          label: `Copy URL of ${app.name}`,
          icon: <CopyGlassIcon size="1.4em" />,
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

  const sections: Record<OverviewSection, ReactNode> = {
    attention: (
      <OverviewSuggestions
        app={app}
        descriptors={suggestions.attention}
        dismiss={suggestions.dismiss}
        latest={latest}
        anchorId="overview-attention"
      />
    ),
    access: (
      <div className="space-y-4">
        <AppAccessCard appName={app.name} />
        <AppAccessDetails app={app} />
      </div>
    ),
    kpis: (
      <div className="space-y-4">
        {isStatic ? null : (
          <section
            aria-label="Live traffic and resources"
            className="grid grid-cols-[repeat(auto-fit,minmax(10.5rem,1fr))] gap-3"
          >
            {layout.containerMetrics ? (
              <ResourceTiles reading={reading} />
            ) : null}
            {layout.traffic !== 'hidden' ? (
              <TrafficTiles appName={app.name} url={url} hideEmpty />
            ) : null}
          </section>
        )}
        {layout.traffic !== 'hidden' ? (
          <VisitorsCard
            appName={app.name}
            window={isStatic ? range : undefined}
            onWindowChange={isStatic ? setRange : undefined}
          />
        ) : null}
      </div>
    ),
    diagnosis: <DiagnosisCard appName={app.name} latest={latest} />,
    health: <AppHealthScorePanel appName={app.name} />,
    activity: (
      <div className="space-y-6">
        {isStatic ? <DropDeployCard appName={app.name} /> : null}
        {uploadAttempt && !isStatic ? (
          <UploadSourceCard
            appName={app.name}
            attempt={uploadAttempt}
            isStatic={isStatic}
          />
        ) : null}
        <div className="grid grid-cols-1 gap-6 *:min-w-0 lg:grid-cols-2">
          <ActivityTimeline
            app={app}
            attempts={attempts}
            conditions={conditions}
          />
          <LogsTail appName={app.name} />
        </div>
      </div>
    ),
    setup: (
      <RecommendedSetup
        app={app}
        hasGitSource={Boolean(git.data)}
        imageDeployed={isImageDeploy(latest?.source)}
        descriptors={suggestions.setup}
        dismissSuggestion={suggestions.dismiss}
        latest={latest}
        onHide={setup.dismiss}
      />
    ),
    details: (
      <DetailsSection app={app} conditions={conditions} isStatic={isStatic} />
    ),
  }
  const serverOutcome = appStatus?.outcome
    ? { outcome: appStatus.outcome, reason: appStatus.outcome_reason }
    : undefined
  const preview = (
    <Suspense
      fallback={<div className="aspect-[8/5] w-full rounded-md bg-muted" />}
    >
      <OverviewPreview
        appName={app.name}
        attempts={attempts}
        url={openUrl}
        isStatic={isStatic}
        onDeploy={setDeployTab}
      />
    </Suspense>
  )
  // Failures stay full width above the grid; the access card moves to the side column.
  const top = layout.sections.filter(
    (id) => id === 'attention' || id === 'diagnosis',
  )
  const main = layout.sections.filter(
    (id) => id !== 'attention' && id !== 'diagnosis' && id !== 'access',
  )
  const hero = (
    <OverviewHero
      app={app}
      conditions={conditions}
      latestAttempt={latest}
      attempts={attempts}
      url={openUrl}
      attentionCount={suggestions.attention.length}
      serverOutcome={serverOutcome}
      onDeploy={setDeployTab}
      gitMissing={git.error instanceof ApiError && git.error.status === 404}
      isStatic={isStatic}
      statStrip={
        isStatic ? (
          <StaticStatStrip appName={app.name} range={range} latest={latest} />
        ) : undefined
      }
      domainCount={app.domains?.length ?? 0}
      recent={
        isStatic ? (
          <RecentDeploys appName={app.name} attempts={attempts} />
        ) : undefined
      }
      preview={isStatic ? preview : undefined}
      summary={
        isStatic ? undefined : (
          <AppSummaryStrip
            app={app}
            attempts={attempts}
            url={openUrl}
            attentionCount={suggestions.attention.length}
          />
        )
      }
    />
  )
  const next = (
    <RecommendedNextCard
      app={app}
      latest={latest}
      omit={[
        'add_domain',
        ...(diagnosisShown ? (['failed_deploy'] as const) : []),
        // A static site has no process to probe.
        ...(isStatic ? (['add_health_check'] as const) : []),
      ]}
    />
  )
  const sheet = (
    <DeploySheet
      appName={app.name}
      tab={deployTab}
      onClose={() => setDeployTab(null)}
    />
  )
  if (isStatic) {
    return (
      <div className="space-y-4">
        {actions.confirmDialog}
        {hero}
        {next}
        {layout.sections.map((id) => (
          <Fragment key={id}>{sections[id]}</Fragment>
        ))}
        {sheet}
      </div>
    )
  }
  return (
    <div className="space-y-3">
      {actions.confirmDialog}
      {hero}
      <OverviewQuickLinks
        app={app}
        reading={reading}
        latest={latest}
        isStatic={isStatic}
      />
      {next}
      {top.map((id) => (
        <Fragment key={id}>{sections[id]}</Fragment>
      ))}
      <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_20rem]">
        <div className="min-w-0 space-y-3">
          <AppWiring
            app={app}
            conditions={conditions}
            latest={latest}
            serverOutcome={serverOutcome}
          />
          <AppDeploys name={app.name} attempts={attempts} />
          {main.map((id) => (
            <Fragment key={id}>{sections[id]}</Fragment>
          ))}
        </div>
        <div className="min-w-0 space-y-3">
          {preview}
          {sections.access}
          <AppRecentPreviews name={app.name} />
        </div>
      </div>
      {sheet}
    </div>
  )
}
