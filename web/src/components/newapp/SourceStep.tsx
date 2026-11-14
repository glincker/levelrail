import { lazy, Suspense, useState } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Link } from '@tanstack/react-router'
import {
  BoxGlassIcon,
  GitBranchGlassIcon,
  LayersGlassIcon,
  RocketGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
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
import { PopularTemplatesRow } from './PopularTemplatesRow'
import { SectionHeading } from './SectionHeading'
import { ErrorBoundary } from '../ErrorBoundary'
import { RepoList } from './RepoList'

const RepoImportFlow = lazy(() =>
  import('./import/RepoImportFlow').then((m) => ({
    default: m.RepoImportFlow,
  })),
)

type SourceChoice = 'git' | 'template' | 'image' | 'sample'

const CHOICES: { key: SourceChoice; icon: UiIcon }[] = [
  { key: 'git', icon: GitBranchGlassIcon },
  { key: 'template', icon: LayersGlassIcon },
  { key: 'image', icon: BoxGlassIcon },
  { key: 'sample', icon: RocketGlassIcon },
]

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
  connectSlot,
}: {
  busy: boolean
  error: string | null
  deploying: boolean
  failure: string | null
  onSource: (source: GitRepoSourceValue) => void
  /** Aside element that hosts the provider card on wide screens. */
  connectSlot?: HTMLElement | null
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
  const [choice, setChoice] = useState<SourceChoice>('git')
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

  const connectCard = noProvider
    ? (() => {
        const card = (
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
        )
        return connectSlot ? createPortal(card, connectSlot) : card
      })()
    : null

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

      <ul
        aria-label={t('source.choose.label')}
        className="grid grid-cols-2 gap-3 md:grid-cols-4"
      >
        {CHOICES.map(({ key, icon: ChoiceIcon }) => (
          <li key={key}>
            <button
              type="button"
              aria-pressed={choice === key}
              onClick={() => {
                setChoice(key)
              }}
              className={cn(
                'flex h-full w-full flex-col items-start gap-2 rounded-container border border-border bg-card p-3 text-left shadow-card transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none motion-reduce:transition-none',
                choice === key &&
                  'border-primary bg-muted/60 ring-1 ring-primary',
              )}
            >
              <ChoiceIcon
                className={cn(
                  'size-7',
                  choice === key ? 'text-foreground' : 'text-muted-foreground',
                )}
                aria-hidden="true"
              />
              <span className="block text-sm font-semibold text-foreground">
                {t(`source.choose.${key}.title`)}
              </span>
              <span className="block text-xs text-muted-foreground">
                {t(`source.choose.${key}.hint`)}
              </span>
            </button>
          </li>
        ))}
      </ul>

      {choice === 'git' ? (
        <div className="space-y-6">
          <section aria-labelledby="new-app-git" className="min-w-0 space-y-3">
            <div className="sr-only">
              <SectionHeading id="new-app-git">
                {noProvider ? t('newAppNoGit:urlTitle') : t('source.git.title')}
              </SectionHeading>
            </div>
            <Suspense fallback={<Skeleton className="h-40 w-full" />}>
              <RepoImportFlow
                accounts={accounts}
                deploying={deploying}
                failure={failure}
                onDeploy={onImport}
                onFallback={onSource}
                hint={noProvider ? t('source.git.noProviderHint') : undefined}
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
                    <ErrorBoundary
                      resetKeys={[repos]}
                      fallback={() => (
                        <Alert role="alert">
                          <AlertDescription>
                            {t('source.repos.crashed')}
                          </AlertDescription>
                        </Alert>
                      )}
                    >
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
                    </ErrorBoundary>
                  ) : null
                }
              />
            </Suspense>
          </section>

          {connectCard}
          <PopularTemplatesRow />
        </div>
      ) : null}

      {choice === 'template' ? <FeaturedTemplates /> : null}

      {choice === 'image' ? <MoreWays onPlan={onPlan} embedded /> : null}

      {choice === 'sample' ? (
        <section
          aria-label={t('source.choose.sample.title')}
          className="flex flex-wrap items-center justify-between gap-3 rounded-container border border-border bg-card p-4 shadow-card"
        >
          <p className="min-w-0 flex-1 text-sm text-muted-foreground">
            {t('source.sample.body')}
          </p>
          <SampleAppButton />
        </section>
      ) : null}
    </div>
  )
}
