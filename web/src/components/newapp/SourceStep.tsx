import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { GitBranchIcon, SquaresFourIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { useConnectedRepos } from '../../hooks/useConnectedRepos'
import { sourceFromRepo } from '../../lib/newApp'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'
import { ImportFrontDoor } from '../ImportFrontDoor'
import { ImportPlanPreview } from '../ImportPlanPreview'
import type { ImportPlan, ImportPlanRequest } from '../../queries/imports'
import { RepoList } from './RepoList'

const URL_PATTERN = /^https?:\/\/[^\s/]+\/\S+$/i

/**
 * Step 1, "Where is it from": a connected repository, a pasted git URL, or
 * anything else (image, compose file, docker run command) through the
 * existing import front door.
 */
export function SourceStep({
  busy,
  error,
  onSource,
}: {
  busy: boolean
  error: string | null
  onSource: (source: GitRepoSourceValue) => void
}) {
  const { t } = useTranslation('newApp')
  const { repos, connectedProviders, isLoading, hasError, providersReady } =
    useConnectedRepos()
  const hasProvider = connectedProviders.length > 0
  const [url, setUrl] = useState('')
  const [branch, setBranch] = useState('')
  const [token, setToken] = useState('')
  const [otherPlan, setOtherPlan] = useState<{
    plan: ImportPlan
    request: ImportPlanRequest
  } | null>(null)

  const urlValid = URL_PATTERN.test(url.trim())

  function submitUrl() {
    if (!urlValid) return
    onSource({
      provider: 'manual',
      repoUrl: url.trim(),
      branch: branch.trim(),
      token: token.trim() || undefined,
    })
  }

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

  if (!providersReady) {
    return (
      <div className="space-y-2" aria-busy="true">
        <Skeleton className="h-8 w-full" />
        <Skeleton className="h-14 w-full" />
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <Tabs defaultValue={hasProvider ? 'repos' : 'url'}>
        <TabsList>
          <TabsTrigger value="repos">{t('source.tabs.repos')}</TabsTrigger>
          <TabsTrigger value="url">{t('source.tabs.url')}</TabsTrigger>
          <TabsTrigger value="other">{t('source.tabs.other')}</TabsTrigger>
          <TabsTrigger value="template">
            {t('source.tabs.template')}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="repos" className="space-y-2 pt-3">
          {hasProvider ? (
            <>
              <RepoList
                repos={repos}
                loading={isLoading}
                onPick={(repo) => {
                  if (!busy) onSource(sourceFromRepo(repo))
                }}
              />
              {hasError ? (
                <p className="text-xs text-muted-foreground">
                  {t('source.repos.partial')}
                </p>
              ) : null}
            </>
          ) : (
            <div className="space-y-3 rounded-md border border-dashed border-border p-4">
              <p className="text-sm font-medium">
                {t('source.repos.emptyTitle')}
              </p>
              <p className="text-sm text-muted-foreground">
                {t('source.repos.emptyBody')}
              </p>
              <Button
                size="sm"
                render={<Link to="/settings/github-app" />}
                nativeButton={false}
              >
                <GitBranchIcon />
                {t('source.repos.connect')}
              </Button>
            </div>
          )}
        </TabsContent>

        <TabsContent value="url" className="space-y-3 pt-3">
          <Field>
            <FieldLabel htmlFor="new-app-url">
              {t('source.url.label')}
            </FieldLabel>
            <Input
              id="new-app-url"
              value={url}
              onChange={(e) => {
                setUrl(e.target.value)
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') submitUrl()
              }}
              placeholder="https://github.com/acme/shop"
              inputMode="url"
              autoComplete="off"
              spellCheck={false}
            />
            <FieldDescription>{t('source.url.hint')}</FieldDescription>
          </Field>
          <Collapsible>
            <CollapsibleTrigger className="text-xs text-muted-foreground underline underline-offset-2">
              {t('source.url.advanced')}
            </CollapsibleTrigger>
            <CollapsiblePanel className="space-y-3 pt-2">
              <Field>
                <FieldLabel htmlFor="new-app-branch">
                  {t('source.url.branch')}
                </FieldLabel>
                <Input
                  id="new-app-branch"
                  value={branch}
                  onChange={(e) => {
                    setBranch(e.target.value)
                  }}
                  placeholder="main"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="new-app-token">
                  {t('source.url.token')}
                </FieldLabel>
                <Input
                  id="new-app-token"
                  type="password"
                  value={token}
                  onChange={(e) => {
                    setToken(e.target.value)
                  }}
                  autoComplete="off"
                />
                <FieldDescription>{t('source.url.tokenHint')}</FieldDescription>
              </Field>
            </CollapsiblePanel>
          </Collapsible>
          <Button
            type="button"
            disabled={!urlValid || busy}
            onClick={submitUrl}
          >
            {busy ? t('source.analyzing') : t('source.analyze')}
          </Button>
        </TabsContent>

        <TabsContent value="other" className="pt-3">
          <ImportFrontDoor onPlan={onPlan} />
        </TabsContent>

        <TabsContent value="template" className="space-y-3 pt-3">
          <p className="text-sm text-muted-foreground">
            {t('source.template.body')}
          </p>
          <Button
            variant="outline"
            render={<Link to="/templates" />}
            nativeButton={false}
          >
            <SquaresFourIcon />
            {t('source.template.browse')}
          </Button>
        </TabsContent>
      </Tabs>
    </div>
  )
}
