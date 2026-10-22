import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Link } from '@tanstack/react-router'
import { useConnectedRepos } from '../../hooks/useConnectedRepos'
import { sourceFromRepo, type NewAppEdits } from '../../lib/newApp'
import type { ImportAnalysis } from '../../queries/importAnalyze'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'
import { ImportPlanPreview } from '../ImportPlanPreview'
import type { ImportPlan, ImportPlanRequest } from '../../queries/imports'
import { NewStaticSiteDrop } from '../dropdeploy/NewStaticSiteDrop'
import { DeployFromComputer } from './DeployFromComputer'
import { FeaturedTemplates } from './FeaturedTemplates'
import { GitProviderCards } from './GitProviderCards'
import { OtherWays } from './OtherWays'
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
  const { t } = useTranslation('newApp')
  const { repos, connectedProviders, isLoading, hasError, providersReady } =
    useConnectedRepos()
  const hasProvider = connectedProviders.length > 0
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
    <div className="space-y-8">
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {PRIVATE_HINT.test(error) ? t('source.privateRepo') : error}
          </AlertDescription>
          {PRIVATE_HINT.test(error) ? (
            <Button
              size="sm"
              className="mt-2"
              render={<Link to="/settings/github-app" />}
              nativeButton={false}
            >
              {t('source.privateRepoConnect')}
            </Button>
          ) : null}
        </Alert>
      ) : null}

      <section aria-labelledby="new-app-git" className="space-y-4">
        <div>
          <h2
            id="new-app-git"
            className="text-base font-medium text-foreground"
          >
            {t('source.git.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('source.git.description')}
          </p>
        </div>
        <Suspense fallback={<Skeleton className="h-40 w-full" />}>
          <RepoImportFlow
            deploying={deploying}
            failure={failure}
            onDeploy={onImport}
            onFallback={onSource}
            renderPicker={(onPick) =>
              !providersReady ? (
                <div className="space-y-2" aria-busy="true">
                  <Skeleton className="h-14 w-full" />
                  <Skeleton className="h-14 w-full" />
                </div>
              ) : hasProvider ? (
                <div className="space-y-2">
                  <p className="text-sm font-medium text-foreground">
                    {t('source.git.pickTitle')}
                  </p>
                  <RepoList
                    repos={repos}
                    loading={isLoading}
                    onPick={(repo) => {
                      if (!busy) onPick(sourceFromRepo(repo))
                    }}
                  />
                  {hasError ? (
                    <p className="text-xs text-muted-foreground">
                      {t('source.repos.partial')}
                    </p>
                  ) : null}
                </div>
              ) : (
                <GitProviderCards />
              )
            }
          />
        </Suspense>
      </section>

      <FeaturedTemplates />
      <OtherWays onPlan={onPlan} />
      <NewStaticSiteDrop />
      <DeployFromComputer />
    </div>
  )
}
