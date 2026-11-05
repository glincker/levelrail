import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Link } from '@tanstack/react-router'
import { useConnectedRepos } from '../../hooks/useConnectedRepos'
import { useRefreshGitRepos } from '../../queries/gitRepos'
import { sourceFromRepo, type NewAppEdits } from '../../lib/newApp'
import type { ImportAnalysis } from '../../queries/importAnalyze'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'
import { ImportPlanPreview } from '../ImportPlanPreview'
import type { ImportPlan, ImportPlanRequest } from '../../queries/imports'
import { FeaturedTemplates } from './FeaturedTemplates'
import { GitProviderCards } from './GitProviderCards'
import { SampleAppButton } from '../getStarted/SampleAppButton'
import { MoreWays } from './MoreWays'
import { SectionHeading } from './SectionHeading'
import { RepoList } from './RepoList'

const RepoImportFlow = lazy(() =>
  import('./import/RepoImportFlow').then((m) => ({
    default: m.RepoImportFlow,
  })),
)

const PRIVATE_HINT = /not found|refused access|cannot see it|token/i

/**
 * Step 1, "Where is it from": import from Git (connected repositories or a
 * pasted URL), start from a featured template, or bring an image, compose
 * file or docker run command through the existing import front door.
 */
export function SourceStep({
  busy,
  error,
  deploying,
  failure,
  onSource,
  onImport,
}: {
  busy: boolean
  error: string | null
  deploying: boolean
  failure: string | null
  onSource: (source: GitRepoSourceValue) => void
  onImport: (
    source: GitRepoSourceValue,
    analysis: ImportAnalysis,
    edits: NewAppEdits,
  ) => void
}) {
  const { t } = useTranslation(['newApp', 'newAppNoGit'])
  const accounts = useConnectedRepos()
  const { repos, connectedProviders, isLoading, hasError, providersReady } =
    accounts
  const hasProvider = connectedProviders.length > 0
  // Without a provider the page leads with what works anyway; connecting is a side note.
  const noProvider = providersReady && !hasProvider
  const refresh = useRefreshGitRepos('')
  const [otherPlan, setOtherPlan] = useState<{
    plan: ImportPlan
    request: ImportPlanRequest
  } | null>(null)

  function onPlan(plan: ImportPlan, request: ImportPlanRequest) {
    if (plan.source === 'repo' && plan.repo_url) {
      onSource({
        provider: 'manual',
        repoUrl: plan.repo_url,
        branch: plan.ref ?? '',
      })
      return
    }
    setOtherPlan({ plan, request })
  }

  if (otherPlan) {
    return (
      <ImportPlanPreview
        plan={otherPlan.plan}
        request={otherPlan.request}
        onBack={() => {
          setOtherPlan(null)
        }}
        onDeployed={() => {
          setOtherPlan(null)
        }}
      />
    )
  }

  return (
    <div className="space-y-6">
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {PRIVATE_HINT.test(error) ? t('source.privateRepo') : error}
          </AlertDescription>
          <Button
            size="sm"
            className="mt-2"
            render={
              <Link
                to={
                  PRIVATE_HINT.test(error)
                    ? '/settings/github-app'
                    : '/settings/git-connections'
                }
              />
            }
            nativeButton={false}
          >
            {PRIVATE_HINT.test(error)
              ? t('source.privateRepoConnect')
              : t('source.errorManage')}
          </Button>
        </Alert>
      ) : null}

      {noProvider ? (
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-xl border border-border bg-card p-3 shadow-xs">
          <p className="min-w-0 flex-1 text-sm text-muted-foreground">
            {t('source.noProvider.lead')}
          </p>
          <SampleAppButton />
        </div>
      ) : null}

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_20rem] xl:items-start">
        <section aria-labelledby="new-app-git" className="min-w-0 space-y-3">
          <SectionHeading id="new-app-git">
            {noProvider ? t('newAppNoGit:urlTitle') : t('source.git.title')}
          </SectionHeading>
          <Suspense fallback={<Skeleton className="h-40 w-full" />}>
            <RepoImportFlow
              accounts={accounts}
              deploying={deploying}
              failure={failure}
              onDeploy={onImport}
              onFallback={onSource}
              renderPicker={(ctx) =>
                ctx.hasSelection ? null : !providersReady ? (
                  <div
                    className="space-y-1.5"
                    role="status"
                    aria-busy="true"
                    aria-label={t('source.repos.loading')}
                  >
                    <Skeleton className="h-11 w-full" />
                    <Skeleton className="h-11 w-full" />
                  </div>
                ) : hasProvider || ctx.urlCandidate ? (
                  <RepoList
                    repos={repos}
                    loading={isLoading}
                    query={ctx.query}
                    urlCandidate={ctx.urlCandidate}
                    hasError={hasError}
                    refreshing={refresh.isPending}
                    onPick={(repo) => {
                      if (!busy) ctx.onPick(sourceFromRepo(repo))
                    }}
                    onUseUrl={(u) => {
                      if (!busy) ctx.onUseUrl(u)
                    }}
                    onRefresh={() => {
                      refresh.mutate()
                    }}
                  />
                ) : null
              }
            />
          </Suspense>
        </section>

        <div className="min-w-0 space-y-4 xl:sticky xl:top-4">
          <FeaturedTemplates />
        </div>
      </div>

      <MoreWays onPlan={onPlan} />

      {noProvider ? (
        <section
          aria-labelledby="new-app-connect"
          className="space-y-2 rounded-xl border border-border bg-card p-3 shadow-xs"
        >
          <h2
            id="new-app-connect"
            className="text-sm font-medium text-foreground"
          >
            {t('newAppNoGit:connect.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('newAppNoGit:connect.reason')}
          </p>
          <GitProviderCards />
        </section>
      ) : null}
    </div>
  )
}
