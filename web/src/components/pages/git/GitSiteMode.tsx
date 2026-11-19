import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { LayoutTemplateGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { BrandIcon } from '../../BrandIcon'
import type { GitRepoSourceValue } from '../../GitRepoSourcePicker'
import { RepoList } from '../../newapp/RepoList'
import { SiteSuccess } from '../SiteSuccess'
import { useConnectedRepos } from '../../../hooks/useConnectedRepos'
import { looksLikeRepoUrl, repoNameFromUrl } from '../../../lib/repoImport'
import { sourceFromRepo } from '../../../lib/newApp'
import { useRefreshGitRepos } from '../../../queries/gitRepos'
import { useFirstDeployResult } from '../../../queries/gitSourceFirstDeploy'
import { createGitSite, type GitSiteResult } from '../../../queries/pagesGit'
import { ApiError } from '../../../lib/apiError'
import { GitSteps } from './GitSteps'
import { previewRepo, repoPreviewKeys } from '../../../queries/repoPreview'
import {
  gitSteps,
  siteNameFromRepo,
  stageFromDeployment,
  staticDetection,
} from './pagesGitLogic'

const rawDebounce: unknown = import.meta.env.VITE_REPO_PREVIEW_DEBOUNCE_MS
const DEBOUNCE_MS = Number(rawDebounce) || 450

function useDebounced(value: string): string {
  const [out, setOut] = useState(value)
  useEffect(() => {
    const id = window.setTimeout(() => setOut(value), DEBOUNCE_MS)
    return () => window.clearTimeout(id)
  }, [value])
  return out
}

interface Props {
  name: string
  nameInvalid: boolean
  onRepoName: (name: string) => void
  onBusyChange: (busy: boolean) => void
  onClose: () => void
}

/** Git mode of the New site dialog: pick a repo, review the detected static build, create and watch the first deploy. */
export default function GitSiteMode({
  name,
  nameInvalid,
  onRepoName,
  onBusyChange,
  onClose,
}: Readonly<Props>) {
  const { t } = useTranslation('pagesSites')
  const navigate = useNavigate()
  const accounts = useConnectedRepos()
  const refresh = useRefreshGitRepos('')
  const [input, setInput] = useState('')
  const [picked, setPicked] = useState<GitRepoSourceValue | null>(null)
  const [branch, setBranch] = useState('')
  const [command, setCommand] = useState<string | null>(null)
  const [outputDir, setOutputDir] = useState<string | null>(null)
  const [forceStatic, setForceStatic] = useState(false)
  const [since, setSince] = useState<string | null>(null)
  const [deployOnPush, setDeployOnPush] = useState(true)
  const [created, setCreated] = useState<GitSiteResult | null>(null)
  const [starting, setStarting] = useState(false)
  const [error, setError] = useState<{
    field: 'repo' | 'output' | null
    message: string
  } | null>(null)

  const typed = useDebounced(input.trim())
  const url = picked ? picked.repoUrl : typed
  const valid = looksLikeRepoUrl(url)
  const preview = useQuery({
    queryKey: repoPreviewKeys.one(
      url,
      branch,
      picked?.providerRef && 'connectionId' in picked.providerRef
        ? picked.providerRef.connectionId
        : '',
      false,
    ),
    queryFn: ({ signal }) =>
      previewRepo(
        {
          repo_url: url,
          ref: branch || undefined,
          account_id:
            picked?.providerRef && 'connectionId' in picked.providerRef
              ? picked.providerRef.connectionId
              : undefined,
        },
        signal,
      ),
    enabled: valid,
    retry: false,
    staleTime: 60_000,
  })
  const data = valid && preview.data?.kind === 'public' ? preview.data : null
  const analysis = data?.analysis
  const detected = useMemo(
    () => (analysis ? staticDetection(analysis) : null),
    [analysis],
  )
  const effectiveBranch =
    branch || data?.default_branch || analysis?.branch || ''
  const repoName = data?.name || repoNameFromUrl(url)

  useEffect(() => {
    if (repoName) onRepoName(siteNameFromRepo(repoName))
  }, [repoName, onRepoName])

  const deploy = useFirstDeployResult(name, since)
  const stage = deploy
    ? stageFromDeployment(deploy.status, deploy.steps?.done ?? 0)
    : since
      ? 'cloning'
      : null
  // The stream wins once it speaks; until then the create response seeds the rows.
  const steps = gitSteps(
    deploy ? deploy.steps?.items : created?.steps?.items,
    deploy ? deploy.status : (created?.status ?? null),
  )
  const failedStep = steps.find((s) => s.state === 'failed')
  const running =
    starting || (stage !== null && stage !== 'failed' && stage !== 'ready')
  useEffect(() => {
    onBusyChange(running)
  }, [running, onBusyChange])

  const buildCommand = command ?? detected?.buildCommand ?? ''
  const publishDir = outputDir ?? detected?.outputDir ?? ''
  const serverApp = detected !== null && !detected.isStatic
  const ready =
    data !== null &&
    analysis !== undefined &&
    !nameInvalid &&
    (!serverApp || forceStatic) &&
    !running

  const reset = (next: string, src: GitRepoSourceValue | null) => {
    setInput(next)
    setPicked(src)
    setBranch('')
    setCommand(null)
    setOutputDir(null)
    setForceStatic(false)
  }

  const create = async () => {
    if (!data) return
    setError(null)
    setStarting(true)
    const ref = picked?.providerRef
    try {
      const result = await createGitSite({
        name,
        repoUrl: data.repo_url ?? url,
        branch: effectiveBranch,
        accountId: ref && 'connectionId' in ref ? ref.connectionId : undefined,
        buildCommand,
        outputDir: publishDir,
        deployOnPush,
      })
      setCreated(result)
      setSince(new Date().toISOString())
    } catch (err) {
      const message = err instanceof Error ? err.message : ''
      // The server undoes a failed create, so the form stays editable for a clean retry.
      const bad = err instanceof ApiError && err.status === 400
      setError({
        field: bad ? (/output/i.test(message) ? 'output' : 'repo') : null,
        message,
      })
    } finally {
      setStarting(false)
    }
  }

  if (stage === 'ready') {
    return <SiteSuccess name={name} onNavigate={onClose} />
  }

  const fixSettings = () => {
    onClose()
    void navigate({
      to: '/pages/$name',
      params: { name },
      search: { tab: 'settings' },
    })
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor="pages-git-repo">{t('git.repo')}</Label>
        <div className="relative">
          <SearchGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            id="pages-git-repo"
            className="pl-8"
            value={picked ? picked.repoUrl : input}
            disabled={running}
            inputMode="url"
            autoComplete="off"
            spellCheck={false}
            placeholder={t('git.repoPlaceholder')}
            aria-invalid={error?.field === 'repo' || undefined}
            aria-describedby={
              error?.field === 'repo' ? 'pages-git-repo-err' : undefined
            }
            onChange={(e) => reset(e.target.value, null)}
          />
        </div>
        {error?.field === 'repo' ? (
          <p
            id="pages-git-repo-err"
            role="alert"
            className="text-xs text-destructive"
          >
            {error.message}
          </p>
        ) : null}
      </div>
      {picked ? null : (
        <RepoList
          repos={accounts.repos}
          loading={accounts.isLoading}
          query={input}
          urlCandidate={looksLikeRepoUrl(input.trim()) ? input.trim() : null}
          hasError={accounts.hasError}
          refreshing={refresh.isPending}
          onPick={(repo) => {
            const src = sourceFromRepo(repo)
            reset(src.repoUrl, src)
          }}
          onUseUrl={(u) =>
            reset(u, { provider: 'manual', repoUrl: u, branch: '' })
          }
          onRefresh={() => refresh.mutate()}
        />
      )}
      {valid && preview.isPending ? (
        <p role="status" className="text-xs text-muted-foreground">
          {t('git.detecting')}
        </p>
      ) : null}
      {valid && (preview.isError || (preview.data && !data)) ? (
        <Alert role="alert">
          <AlertDescription>{t('git.unreadable')}</AlertDescription>
        </Alert>
      ) : null}
      {data && analysis && detected ? (
        <section
          aria-label={t('git.detected')}
          className="space-y-3 rounded-md border border-border bg-muted/30 p-3"
        >
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="flex min-w-0 items-center gap-2 text-sm font-medium">
              {detected.logo ? (
                <BrandIcon name={detected.logo} className="size-5 shrink-0" />
              ) : (
                <LayoutTemplateGlassIcon
                  className="size-5 shrink-0 text-muted-foreground"
                  aria-hidden="true"
                />
              )}
              <span className="truncate">
                {detected.framework || t('git.plainHtml')}
              </span>
            </p>
            <span className="flex items-center gap-2">
              <span className="rounded-full border border-border px-2 py-0.5 text-xs">
                {detected.isStatic || forceStatic
                  ? t('git.static')
                  : t('git.notStatic')}
              </span>
              <span className="flex items-center gap-1.5">
                <Label htmlFor="pages-git-branch" className="text-xs">
                  {t('git.branch')}
                </Label>
                <NativeSelect
                  id="pages-git-branch"
                  value={effectiveBranch}
                  disabled={running}
                  onChange={(e) => {
                    setBranch(e.target.value)
                    setCommand(null)
                    setOutputDir(null)
                  }}
                >
                  {(data.branches?.length
                    ? data.branches
                    : [effectiveBranch]
                  ).map((b) => (
                    <option key={b} value={b}>
                      {b}
                    </option>
                  ))}
                </NativeSelect>
              </span>
            </span>
          </div>
          {serverApp ? (
            <Alert role="alert">
              <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
                <span>{t('git.serverWarning')}</span>
                <span className="flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    render={<Link to="/apps/new" />}
                  >
                    {t('git.useAppFlow')}
                  </Button>
                  {forceStatic ? null : (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setForceStatic(true)}
                    >
                      {t('git.forceStatic')}
                    </Button>
                  )}
                </span>
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="grid gap-3 min-[420px]:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="pages-git-command">{t('git.command')}</Label>
              <Input
                id="pages-git-command"
                className="font-mono"
                value={buildCommand}
                disabled={running}
                placeholder={t('git.commandPlaceholder')}
                onChange={(e) => setCommand(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="pages-git-output">{t('git.output')}</Label>
              <Input
                id="pages-git-output"
                className="font-mono"
                value={publishDir}
                disabled={running}
                placeholder={t('git.outputPlaceholder')}
                aria-invalid={error?.field === 'output' || undefined}
                aria-describedby={
                  error?.field === 'output' ? 'pages-git-output-err' : undefined
                }
                onChange={(e) => setOutputDir(e.target.value)}
              />
              {error?.field === 'output' ? (
                <p
                  id="pages-git-output-err"
                  role="alert"
                  className="text-xs text-destructive"
                >
                  {error.message}
                </p>
              ) : null}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Switch
              id="pages-git-push"
              size="sm"
              checked={deployOnPush}
              disabled={running}
              onCheckedChange={setDeployOnPush}
            />
            <Label htmlFor="pages-git-push" className="text-sm font-normal">
              {t('git.deployOnPush')}
            </Label>
          </div>
        </section>
      ) : null}
      {stage !== null ? <GitSteps steps={steps} /> : null}
      {error && error.field === null ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>
            {t('git.createFailed', { message: error.message })}
          </AlertDescription>
        </Alert>
      ) : null}
      {stage === 'failed' ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
            <span>
              {t('git.failedStep', {
                step: t(
                  `git.steps.${failedStep?.name ?? deploy?.steps?.failing_step ?? 'building'}`,
                  {
                    defaultValue: deploy?.steps?.failing_step ?? '',
                  },
                ),
                message: deploy?.error_summary ?? '',
              })}
            </span>
            <span className="flex gap-2">
              {deploy ? (
                <Button
                  size="sm"
                  variant="outline"
                  render={
                    <Link
                      to="/apps/$name/deploys/$deployId/logs"
                      params={{ name, deployId: deploy.id }}
                      onClick={onClose}
                    />
                  }
                >
                  {t('git.viewLog')}
                </Button>
              ) : null}
              <Button size="sm" onClick={fixSettings}>
                {t('git.fixSettings')}
              </Button>
            </span>
          </AlertDescription>
        </Alert>
      ) : null}
      {stage !== null && stage !== 'failed' && (deploy ?? created?.deployId) ? (
        <Link
          to="/apps/$name/deploys/$deployId/logs"
          params={{ name, deployId: deploy?.id ?? created?.deployId ?? '' }}
          onClick={onClose}
          className="text-xs text-muted-foreground underline-offset-2 hover:underline focus-visible:underline"
        >
          {t('git.viewLog')}
        </Link>
      ) : null}
      <div className="flex justify-end">
        <Button
          type="button"
          size="sm"
          disabled={!ready}
          onClick={() => void create()}
        >
          {running ? t('new.creating') : t('git.create')}
        </Button>
      </div>
    </div>
  )
}
