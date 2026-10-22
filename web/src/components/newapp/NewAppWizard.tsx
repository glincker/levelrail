import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import {
  analyzeImport,
  type ImportAnalysis,
  type ImportAnalyzeRequest,
} from '../../queries/importAnalyze'
import { appKeys } from '../../queries/apps'
import {
  ApplyStepError,
  applyNewApp,
  editsFromAnalysis,
  type ApplyStep,
  type NewAppEdits,
} from '../../lib/newApp'
import {
  clearImportDraft,
  loadImportDraft,
  saveImportDraft,
} from '../../lib/importDraft'
import type { GitRepoSourceValue } from '../GitRepoSourcePicker'
import { SourceStep } from './SourceStep'
import { FindingsStep } from './FindingsStep'
import { ReviewStep } from './ReviewStep'

type StepId = 'source' | 'findings' | 'review'
const STEPS: StepId[] = ['source', 'findings', 'review']

interface Failure {
  step: ApplyStep
  message: string
  appCreated: boolean
}

/**
 * The suggestion-driven "New app" flow: where it is from, what we found,
 * review and deploy. The analysis is read-only; nothing exists until the
 * final Deploy.
 */
export function NewAppWizard() {
  const { t } = useTranslation('newApp')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [draft] = useState(loadImportDraft)
  const [step, setStep] = useState<StepId>('source')
  const [source, setSource] = useState<GitRepoSourceValue | null>(
    draft && draft.step !== 'source' ? draft.source : null,
  )
  const [analysis, setAnalysis] = useState<ImportAnalysis | null>(null)
  const [edits, setEdits] = useState<NewAppEdits | null>(null)
  const [failure, setFailure] = useState<Failure | null>(null)
  const resumeStep = useRef<StepId | null>(draft?.step ?? null)

  const analyze = useMutation({
    mutationFn: (req: ImportAnalyzeRequest) => analyzeImport(req),
  })

  const deploy = useMutation({
    mutationFn: (job: DeployJob) =>
      applyNewApp(job.analysis, job.source, job.edits, job.resume),
    onSuccess: (result) => {
      clearImportDraft()
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      toast.add({
        title: t('toast.created', { name: result.appName }),
        description: t('toast.watching'),
        type: 'success',
      })
      void navigate({
        to: '/apps/$name/deploys/$deployId/logs',
        params: { name: result.appName, deployId: result.deployId },
      })
    },
    onError: (err) => {
      if (err instanceof ApplyStepError) {
        setFailure({
          step: err.step,
          message: err.message,
          appCreated: err.appCreated,
        })
      } else {
        setFailure({
          step: 'app',
          message: err instanceof Error ? err.message : String(err),
          appCreated: false,
        })
      }
    },
  })

  function run(
    src: GitRepoSourceValue,
    override: Partial<ImportAnalyzeRequest> = {},
    keepEnv: Record<string, string> = {},
  ) {
    setSource(src)
    runAnalysis(src, override, keepEnv)
  }

  function runAnalysis(
    src: GitRepoSourceValue,
    override: Partial<ImportAnalyzeRequest>,
    keepEnv: Record<string, string>,
  ) {
    analyze.mutate(
      {
        repo_url: src.repoUrl,
        ref: src.branch || undefined,
        token: src.token,
        ...override,
      },
      {
        onSuccess: (result) => {
          setAnalysis(result)
          setEdits({ ...editsFromAnalysis(result), env: keepEnv })
          setStep(resumeStep.current ?? 'findings')
          resumeStep.current = null
        },
      },
    )
  }

  useEffect(() => {
    if (!draft || draft.step === 'source') return
    runAnalysis(
      draft.source,
      draft.baseDirectory ? { base_directory: draft.baseDirectory } : {},
      {},
    )
    // restore once on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!source || step === 'source') return
    const safe = {
      provider: source.provider,
      repoUrl: source.repoUrl,
      branch: source.branch,
      providerRef: source.providerRef,
    }
    saveImportDraft({
      step,
      source: safe,
      baseDirectory: edits?.baseDirectory ?? '',
    })
  }, [source, step, edits?.baseDirectory])

  const current = STEPS.indexOf(step)

  return (
    <div className="mx-auto w-full max-w-3xl space-y-5">
      {step !== 'source' ? (
        <ol
          className="flex flex-wrap gap-2 text-sm"
          aria-label={t('steps.label')}
        >
          {STEPS.map((s, i) => (
            <li
              key={s}
              aria-current={i === current ? 'step' : undefined}
              className={cn(
                'flex items-center gap-2 rounded-md border px-2.5 py-1',
                i === current
                  ? 'border-primary/50 bg-muted font-medium text-foreground'
                  : 'border-border text-muted-foreground',
              )}
            >
              <span className="font-mono text-xs">{i + 1}</span>
              {t(`steps.${s}`)}
            </li>
          ))}
        </ol>
      ) : null}

      {step === 'source' ? (
        <>
          {analyze.isPending ? (
            <AnalyzingSkeleton label={t('source.analyzing')} />
          ) : null}
          <div className={analyze.isPending ? 'hidden' : undefined}>
            <SourceStep
              busy={analyze.isPending}
              error={analyze.isError ? analyze.error.message : null}
              deploying={deploy.isPending}
              failure={failure?.message ?? null}
              onSource={(src) => {
                run(src)
              }}
              onImport={(src, a, e) => {
                setFailure(null)
                setSource(src)
                setAnalysis(a)
                setEdits(e)
                deploy.mutate({
                  analysis: a,
                  source: src,
                  edits: e,
                  resume: failure?.appCreated === true,
                })
              }}
            />
          </div>
        </>
      ) : null}

      {step === 'findings' && analysis && edits && source ? (
        analyze.isPending ? (
          <AnalyzingSkeleton label={t('source.analyzing')} />
        ) : (
          <FindingsStep
            analysis={analysis}
            source={source}
            edits={edits}
            onEdits={setEdits}
            onReanalyze={(override) => {
              run(source, override, edits.env)
            }}
            onBack={() => {
              clearImportDraft()
              setStep('source')
            }}
            onNext={() => {
              setFailure(null)
              setStep('review')
            }}
          />
        )
      ) : null}

      {step === 'review' && analysis && edits ? (
        <ReviewStep
          analysis={analysis}
          edits={edits}
          deploying={deploy.isPending}
          failure={failure}
          onBack={() => {
            setStep('findings')
          }}
          onDeploy={() => {
            setFailure(null)
            if (analysis && source && edits) {
              deploy.mutate({
                analysis,
                source,
                edits,
                resume: failure?.appCreated === true,
              })
            }
          }}
        />
      ) : null}
    </div>
  )
}

interface DeployJob {
  analysis: ImportAnalysis
  source: GitRepoSourceValue
  edits: NewAppEdits
  resume: boolean
}

function AnalyzingSkeleton({ label }: { label: string }) {
  return (
    <div className="space-y-3" role="status" aria-live="polite">
      <p className="text-sm text-muted-foreground">{label}</p>
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-10 w-full" />
      <Skeleton className="h-24 w-full" />
    </div>
  )
}
