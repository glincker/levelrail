import { useCallback, useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { DraftBanner } from '../../DraftBanner'
import { useDraft } from '../../../hooks/useDraft'
import { domainsQueryOptions } from '../../../queries/domains'
import {
  useDeployFunction,
  useFunctionRuntimes,
} from '../../../queries/functions'
import { DeployResult } from './DeployResult'
import { DeploySummary } from './DeploySummary'
import {
  DEFAULT_IDLE_MINUTES,
  FALLBACK_PORT,
  idleMinutes,
  normalizePath,
  previewUrls,
  suggestName,
  validate,
  type FieldErrors,
  type FlowValues,
} from './logic'
import { findPreset, presetDefaults, PRESETS } from './presets'
import { StepDetails } from './StepDetails'
import { StepSource } from './StepSource'
import { StepTemplate } from './StepTemplate'
import { useNameState } from './useNameTaken'

type Step = 'template' | 'source' | 'details' | 'result'
type ShownStep = Exclude<Step, 'result'>
const STEP_ORDER: ShownStep[] = ['template', 'source', 'details']
const SOURCE_FIELDS: (keyof FlowValues)[] = ['repoUrl', 'image']
const DEFAULT_REF = 'main'

function initialValues(): FlowValues {
  return {
    source: 'repo',
    repoUrl: '',
    ref: DEFAULT_REF,
    directory: '',
    image: '',
    name: '',
    port: String(FALLBACK_PORT),
    idleChoice: String(DEFAULT_IDLE_MINUTES) === '10' ? '10' : 'custom',
    customIdle: String(DEFAULT_IDLE_MINUTES),
    routeDomain: '',
    routePath: '',
  }
}

function pick(errors: FieldErrors, keys: (keyof FlowValues)[]): FieldErrors {
  const out: FieldErrors = {}
  for (const k of keys) if (errors[k]) out[k] = errors[k]
  return out
}

export function DeployFlow({ onClose }: Readonly<{ onClose: () => void }>) {
  const { t } = useTranslation('functions')
  const [step, setStep] = useState<Step>('template')
  const [presetId, setPresetId] = useState(PRESETS[0]?.id ?? 'go')
  const draft = useDraft('function-deploy', initialValues())
  const values = draft.value
  const setValues = draft.setValue
  const [nameTouched, setNameTouched] = useState(false)
  const [portTouched, setPortTouched] = useState(false)
  const [showErrors, setShowErrors] = useState(false)
  const [deployedName, setDeployedName] = useState('')
  const deploy = useDeployFunction()
  const runtimes = useFunctionRuntimes()
  const domains = useQuery(domainsQueryOptions()).data ?? []
  const preset = findPreset(presetId)

  const update = useCallback((patch: Partial<FlowValues>) => {
    setValues((prev) => ({ ...prev, ...patch }))
    if (patch.port !== undefined) setPortTouched(true)
  }, [])

  const updateSource = useCallback(
    (patch: Partial<FlowValues>) => {
      setValues((prev) => {
        const next = { ...prev, ...patch }
        if (
          !nameTouched &&
          (patch.repoUrl !== undefined || patch.image !== undefined)
        ) {
          next.name = suggestName({
            source: next.source,
            repoUrl: next.repoUrl,
            image: next.image,
            presetName: findPreset(presetId).suggestedName,
          })
        }
        return next
      })
    },
    [nameTouched, presetId],
  )

  const onDetectedPort = useCallback(
    (port: number) => {
      if (!portTouched) setValues((prev) => ({ ...prev, port: String(port) }))
    },
    [portTouched],
  )

  function choosePreset(id: string) {
    const p = findPreset(id)
    setPresetId(p.id)
    setNameTouched(false)
    setPortTouched(false)
    setShowErrors(false)
    setValues({
      ...initialValues(),
      ...presetDefaults(p, runtimes.data?.port),
    })
    setStep('source')
  }

  const nameValid = !validate(values).name
  const nameState = useNameState(values.name, nameValid)
  const errors = validate(values, nameState === 'taken')
  const preview = previewUrls({
    name: values.name,
    routeDomain: values.routeDomain,
    routePath: values.routePath,
    domains,
  })

  function goNext() {
    if (Object.keys(pick(errors, SOURCE_FIELDS)).length > 0) {
      setShowErrors(true)
      return
    }
    setShowErrors(false)
    setStep('details')
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (Object.keys(errors).length > 0 || nameState === 'checking') {
      setShowErrors(true)
      return
    }
    const name = values.name.trim()
    const path = normalizePath(values.routePath)
    deploy.mutate(
      {
        name,
        repoUrl: values.source === 'repo' ? values.repoUrl.trim() : '',
        ref: values.ref.trim() || DEFAULT_REF,
        directory: values.directory.trim(),
        image: values.source === 'image' ? values.image.trim() : undefined,
        port: Number(values.port),
        idleMinutes: idleMinutes(values),
        routeDomain: path ? values.routeDomain : '',
        routePath: path,
      },
      {
        onSuccess: () => {
          draft.clearOnSuccess()
          setDeployedName(name)
          setStep('result')
        },
        onError: (err) => {
          toast.add({
            title: t('deploy.failed', { name, message: err.message }),
            type: 'error',
          })
        },
      },
    )
  }

  if (step === 'result') {
    return (
      <DeployResult
        name={deployedName}
        fallbackUrl={preview.routeUrl ?? preview.functionUrl}
        onClose={onClose}
      />
    )
  }

  const index = STEP_ORDER.indexOf(step)
  const shown = showErrors ? errors : {}

  return (
    <form
      noValidate
      onSubmit={onSubmit}
      className="flex min-h-0 min-w-0 flex-1 flex-col gap-4"
    >
      <DialogHeader>
        <DialogTitle>{t('deploy.title')}</DialogTitle>
        <DialogDescription>{t('deploy.description')}</DialogDescription>
      </DialogHeader>
      <ol className="hidden flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground sm:flex">
        {STEP_ORDER.map((s, i) => (
          <li
            key={s}
            aria-current={s === step ? 'step' : undefined}
            className={s === step ? 'font-semibold text-foreground' : undefined}
          >
            {i + 1}. {t(`deploy.steps.${s}`)}
          </li>
        ))}
      </ol>
      <p className="text-xs font-medium text-muted-foreground sm:sr-only">
        {t('deploy.stepLabel', {
          current: index + 1,
          total: STEP_ORDER.length,
        })}
      </p>
      <div className="min-h-0 min-w-0 -mx-1 flex-1 space-y-4 overflow-y-auto px-1">
        <DraftBanner
          draft={draft}
          onRestore={() => {
            setNameTouched(true)
            setStep('source')
          }}
        />
        {step === 'template' ? (
          <StepTemplate selectedId={presetId} onSelect={choosePreset} />
        ) : null}
        {step === 'source' ? (
          <StepSource
            preset={preset}
            values={values}
            errors={shown}
            update={updateSource}
            onDetectedPort={onDetectedPort}
          />
        ) : null}
        {step === 'details' ? (
          <>
            <StepDetails
              values={values}
              errors={shown}
              domains={domains}
              preview={preview}
              nameState={nameState}
              update={update}
              onName={(name) => {
                setNameTouched(true)
                update({ name })
              }}
            />
            <DeploySummary values={values} preview={preview} />
          </>
        ) : null}
      </div>
      <DialogFooter className="sm:flex-wrap">
        <Button type="button" variant="outline" onClick={onClose}>
          {t('deploy.cancel')}
        </Button>
        {index > 0 ? (
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setShowErrors(false)
              setStep(STEP_ORDER[index - 1] ?? 'template')
            }}
          >
            {t('deploy.back')}
          </Button>
        ) : null}
        {step === 'source' ? (
          <Button type="button" onClick={goNext}>
            {t('deploy.next')}
          </Button>
        ) : null}
        {step === 'details' ? (
          <Button type="submit" disabled={deploy.isPending}>
            {deploy.isPending ? t('deploy.submitting') : t('deploy.submit')}
          </Button>
        ) : null}
      </DialogFooter>
    </form>
  )
}
