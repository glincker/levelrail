import { useState } from 'react'
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
  const [step, setStep] = useState<StepId>('source')
  const [source, setSource] = useState<GitRepoSourceValue | null>(null)
  const [analysis, setAnalysis] = useState<ImportAnalysis | null>(null)
  const [edits, setEdits] = useState<NewAppEdits | null>(null)
  const [failure, setFailure] = useState<Failure | null>(null)

  const analyze = useMutation({
    mutationFn: (req: ImportAnalyzeRequest) => analyzeImport(req),
  })

  const deploy = useMutation({
    mutationFn: (resume: boolean) => {
      if (!analysis || !source || !edits) {
        return Promise.reject(new Error('nothing to deploy'))
      }
      return applyNewApp(analysis, source, edits, resume)
    },
    onSuccess: (result) => {
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
          setStep('findings')
        },
      },
    )
  }

  const current = STEPS.indexOf(step)

  return (
    <div className="mx-auto w-full max-w-3xl space-y-5">
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

      {step === 'source' ? (
        <>
          {analyze.isPending ? (
            <AnalyzingSkeleton label={t('source.analyzing')} />
          ) : null}
          <div className={analyze.isPending ? 'hidden' : undefined}>
            <SourceStep
              busy={analyze.isPending}
              error={analyze.isError ? analyze.error.message : null}
              onSource={(src) => {
                run(src)
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
            edits={edits}
            onEdits={setEdits}
            onReanalyze={(override) => {
              run(source, override, edits.env)
            }}
            onBack={() => {
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
            deploy.mutate(failure?.appCreated === true)
          }}
        />
      ) : null}
    </div>
  )
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
