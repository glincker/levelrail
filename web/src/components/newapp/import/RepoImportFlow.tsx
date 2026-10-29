import { DraftBanner } from '../../DraftBanner'
import { useDraft } from '../../../hooks/useDraft'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { SearchGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useAppListOptional } from '../../../queries/apps'
import { analyzeImport } from '../../../queries/importAnalyze'
import {
  previewRepo,
  repoPreviewKeys,
  type RepoPreview,
} from '../../../queries/repoPreview'
import {
  hostBrand,
  looksLikeRepoUrl,
  repoNameFromUrl,
  suggestAppName,
} from '../../../lib/repoImport'
import type { NewAppEdits } from '../../../lib/newApp'
import type { ImportAnalysis } from '../../../queries/importAnalyze'
import type { GitRepoSourceValue } from '../../GitRepoSourcePicker'
import type { ConnectedRepos } from '../../../hooks/useConnectedRepos'
import { DEFAULT_DEPLOY_OPTIONS } from '../../../lib/deployOptions'
import { GitSettings, type ImportDefaults } from '../GitSettings'
import { DetectedStack } from './DetectedStack'
import { PreviewNotice } from './PreviewNotice'
import { PreviewSkeleton, RepoPreviewCard } from './RepoPreviewCard'
import { RepoSimpleForm } from './RepoSimpleForm'

const rawDebounce: unknown = import.meta.env.VITE_REPO_PREVIEW_DEBOUNCE_MS
const DEBOUNCE_MS = Number(rawDebounce) || 450

function useDebounced(value: string): string {
  const [out, setOut] = useState(value)
  useEffect(() => {
    const id = window.setTimeout(() => {
      setOut(value)
    }, DEBOUNCE_MS)
    return () => {
      window.clearTimeout(id)
    }
  }, [value])
  return out
}

/** What the picker needs to draw the one repository list under the field. */
export interface PickerContext {
  query: string
  /** A pasted repository URL, when the text parses as one. */
  urlCandidate: string | null
  hasSelection: boolean
  onPick: (source: GitRepoSourceValue) => void
  onUseUrl: (url: string) => void
}

const URL_ATTEMPT = /^(?:https?:|ssh:|git:|[\w.-]+@)/i

function accountOf(src: GitRepoSourceValue | null): string {
  const ref = src?.providerRef
  return ref && 'connectionId' in ref ? ref.connectionId : ''
}

/**
 * One page, three calm steps: where the code is, a repository preview, and a
 * short form. Picking a repo or pasting a URL previews it with no button.
 */
export function RepoImportFlow({
  accounts,
  renderPicker,
  deploying,
  failure,
  onDeploy,
  onFallback,
}: {
  accounts: ConnectedRepos
  renderPicker: (ctx: PickerContext) => ReactNode
  deploying: boolean
  failure: string | null
  onDeploy: (
    source: GitRepoSourceValue,
    analysis: ImportAnalysis,
    edits: NewAppEdits,
  ) => void
  onFallback: (source: GitRepoSourceValue) => void
}) {
  const { t } = useTranslation('newApp')
  const draft = useDraft('repo-import-url', { input: '' })
  const input = draft.value.input
  const setInput = (v: string) => draft.update({ input: v })
  const [picked, setPicked] = useState<GitRepoSourceValue | null>(null)
  const [token, setToken] = useState('')
  const [branch, setBranch] = useState('')
  const [baseDirectory, setBaseDirectory] = useState('')
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [deployOnPush, setDeployOnPush] = useState(
    DEFAULT_DEPLOY_OPTIONS.deployOnPush,
  )

  const typed = useDebounced(input.trim())
  const url = picked ? picked.repoUrl : typed
  const valid = looksLikeRepoUrl(url)
  const accountId = accountOf(picked)

  const preview = useQuery({
    queryKey: repoPreviewKeys.one(url, branch, accountId, token !== ''),
    queryFn: ({ signal }) =>
      previewRepo(
        {
          repo_url: url,
          ref: branch || undefined,
          account_id: accountId || undefined,
          token: token || undefined,
        },
        signal,
      ),
    enabled: valid,
    retry: false,
    staleTime: 60_000,
    placeholderData: (prev, prevQuery) =>
      prevQuery?.queryKey[1] === url ? prev : undefined,
  })
  const data: RepoPreview | undefined = valid ? preview.data : undefined
  const baseAnalysis = data?.analysis
  const mono = useQuery({
    queryKey: ['repo-preview-mono', url, branch, baseDirectory, token !== ''],
    queryFn: () =>
      analyzeImport({
        repo_url: url,
        ref: branch || data?.default_branch,
        base_directory: baseDirectory,
        token: token || undefined,
      }),
    enabled: baseAnalysis !== undefined && baseDirectory !== '',
    retry: false,
  })
  const analysis =
    baseDirectory !== '' ? (mono.data ?? baseAnalysis) : baseAnalysis

  const apps = useAppListOptional()
  const taken = useMemo(
    () => new Set((apps.data ?? []).map((a) => a.name)),
    [apps.data],
  )

  const effectiveBranch =
    branch || data?.default_branch || analysis?.branch || ''
  const source: GitRepoSourceValue = {
    provider: picked?.provider ?? 'manual',
    repoUrl: data?.repo_url ?? url,
    branch: effectiveBranch,
    token: token || undefined,
    providerRef: picked?.providerRef,
  }
  const initialName = suggestAppName(
    analysis?.name.value || data?.name || repoNameFromUrl(url),
  )
  const formKey = `${data?.repo_url ?? url}|${baseDirectory}`

  const typedInput = input.trim()
  const urlCandidate =
    !picked && looksLikeRepoUrl(typedInput) ? typedInput : null
  const urlLooksBroken =
    !picked && URL_ATTEMPT.test(typedInput) && !looksLikeRepoUrl(typedInput)
  const publicPreview = data?.kind === 'public' ? data : undefined
  const packages =
    analysis?.monorepo && (analysis.packages?.length ?? 0) > 1
      ? (analysis.packages ?? [])
      : []
  const defaults: ImportDefaults | null = publicPreview
    ? {
        branch: effectiveBranch,
        branches: publicPreview.branches ?? [],
        onBranch: (b) => {
          setBranch(b)
          setBaseDirectory('')
        },
        packages,
        baseDirectory,
        onBaseDirectory: setBaseDirectory,
        deployOnPush,
        onDeployOnPush: setDeployOnPush,
      }
    : null

  function reset(next: string, src: GitRepoSourceValue | null) {
    setInput(next)
    setPicked(src)
    setBranch('')
    setBaseDirectory('')
    setToken('')
  }

  return (
    <div className="min-w-0 space-y-5">
      <DraftBanner draft={draft} />
      <Field>
        <div className="flex items-center justify-between gap-2">
          <FieldLabel htmlFor="new-app-url">
            {t('source.find.label')}
          </FieldLabel>
          <GitSettings
            accounts={accounts}
            defaults={defaults}
            open={settingsOpen}
            onOpenChange={setSettingsOpen}
          />
        </div>
        <div className="relative">
          <SearchGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-4.5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            id="new-app-url"
            value={picked ? picked.repoUrl : input}
            onChange={(e) => {
              reset(e.target.value, null)
            }}
            placeholder={t('source.find.placeholder')}
            inputMode="url"
            autoComplete="off"
            spellCheck={false}
            className="pr-9 pl-8"
            aria-invalid={urlLooksBroken}
          />
          {input.trim() !== '' || picked ? (
            <Button
              type="button"
              size="icon-xs"
              variant="ghost"
              className="absolute top-1/2 right-1.5 -translate-y-1/2"
              aria-label={t('source.find.clear')}
              onClick={() => {
                reset('', null)
              }}
            >
              <XGlassIcon size="1.2em" aria-hidden="true" />
            </Button>
          ) : null}
        </div>
        <FieldDescription>
          {urlLooksBroken ? t('import.urlInvalid') : t('source.find.hint')}
        </FieldDescription>
      </Field>
      {renderPicker({
        query: input,
        urlCandidate,
        hasSelection: picked !== null,
        onPick: (src) => {
          reset(src.repoUrl, src)
        },
        onUseUrl: (u) => {
          reset(u, { provider: 'manual', repoUrl: u, branch: '' })
        },
      })}

      {valid && preview.isPending ? (
        <PreviewSkeleton label={t('import.loading')} />
      ) : null}
      {valid && preview.isError ? (
        <PreviewNotice
          kind="rate_limited"
          brand="git"
          onToken={setToken}
          onContinue={() => {
            onFallback(source)
          }}
        />
      ) : null}
      {data && data.kind !== 'public' && data.kind !== 'invalid_url' ? (
        <PreviewNotice
          kind={data.kind}
          brand={hostBrand(data.host)}
          onToken={setToken}
          onContinue={() => {
            onFallback(source)
          }}
        />
      ) : null}
      {data?.kind === 'public' ? (
        <RepoPreviewCard preview={data} branch={effectiveBranch}>
          {analysis ? (
            <DetectedStack
              analysis={analysis}
              baseDirectory={baseDirectory}
              onChooseApp={() => {
                setSettingsOpen(true)
              }}
              onEdit={() => {
                setAdvancedOpen(true)
              }}
            />
          ) : (
            <PreviewNotice
              kind="unsupported_host"
              brand={hostBrand(data.host)}
              onToken={setToken}
              onContinue={() => {
                onFallback(source)
              }}
            />
          )}
        </RepoPreviewCard>
      ) : null}
      {data?.kind === 'public' && analysis ? (
        <RepoSimpleForm
          key={formKey}
          analysis={analysis}
          branch={effectiveBranch}
          initialName={initialName}
          deployOnPush={deployOnPush}
          takenNames={taken}
          deploying={deploying}
          failure={failure}
          advancedOpen={advancedOpen}
          onAdvancedOpen={setAdvancedOpen}
          onSubmit={(edits) => {
            onDeploy(source, analysis, edits)
          }}
        />
      ) : null}
    </div>
  )
}
