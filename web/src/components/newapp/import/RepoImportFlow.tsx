import { DraftBanner } from '../../DraftBanner'
import { useDraft } from '../../../hooks/useDraft'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
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

function accountOf(src: GitRepoSourceValue | null): string {
  const ref = src?.providerRef
  return ref && 'connectionId' in ref ? ref.connectionId : ''
}

/**
 * One page, three calm steps: where the code is, a repository preview, and a
 * short form. Picking a repo or pasting a URL previews it with no button.
 */
export function RepoImportFlow({
  renderPicker,
  deploying,
  failure,
  onDeploy,
  onFallback,
}: {
  renderPicker: (onPick: (source: GitRepoSourceValue) => void) => ReactNode
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
        <FieldLabel htmlFor="new-app-url">{t('source.url.label')}</FieldLabel>
        <Input
          id="new-app-url"
          value={picked ? picked.repoUrl : input}
          onChange={(e) => {
            reset(e.target.value, null)
          }}
          placeholder="https://github.com/acme/shop"
          inputMode="url"
          autoComplete="off"
          spellCheck={false}
          aria-invalid={input.trim() !== '' && !looksLikeRepoUrl(input.trim())}
        />
        <FieldDescription>
          {input.trim() !== '' && !looksLikeRepoUrl(input.trim())
            ? t('import.urlInvalid')
            : t('source.url.hint')}
        </FieldDescription>
      </Field>
      {renderPicker((src) => {
        reset(src.repoUrl, src)
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
        <RepoPreviewCard
          preview={data}
          branch={effectiveBranch}
          onBranch={(b) => {
            setBranch(b)
            setBaseDirectory('')
          }}
        >
          {analysis ? (
            <DetectedStack
              analysis={analysis}
              baseDirectory={baseDirectory}
              onBaseDirectory={setBaseDirectory}
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
