import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  classifyDomain,
  clearWizardState,
  loadWizardState,
  normalizeDomainInput,
  saveWizardState,
  type DomainKind,
  type WizardState,
  type WizardStep,
} from '../../lib/domainWizard'
import { useEditAppDomains } from '../../queries/appEnvironmentDomains'
import { LiveStep, PointStep } from './AddDomainWizardSteps'

const STEPS: WizardStep[] = [1, 2, 3]

function Progress({ step }: { step: WizardStep }) {
  const { t } = useTranslation('domains')
  return (
    <div className="space-y-1.5">
      <p className="text-xs text-muted-foreground" aria-live="polite">
        {t('addWizard.progress', { current: step, total: STEPS.length })}
        {': '}
        {t(`addWizard.stepNames.${step}`)}
      </p>
      <ol className="grid grid-cols-3 gap-1.5" aria-hidden="true">
        {STEPS.map((s) => (
          <li
            key={s}
            className={
              s <= step
                ? 'h-1 rounded-full bg-primary'
                : 'h-1 rounded-full bg-muted'
            }
          />
        ))}
      </ol>
    </div>
  )
}

export function AddDomainWizard({
  apps,
  claimedBy,
  initialApp,
  onClose,
  onAdvanced,
}: {
  apps: { name: string }[]
  claimedBy: ReadonlyMap<string, string>
  initialApp?: string
  onClose: () => void
  onAdvanced: () => void
}) {
  const { t } = useTranslation('domains')
  const [state, setState] = useState<WizardState>(() => {
    const saved = loadWizardState()
    // A saved app that no longer exists must not survive a reload.
    const valid =
      saved !== null &&
      apps.some((a) => a.name === saved.app) &&
      (saved.step === 1 || saved.added !== '')
    if (saved && valid) return saved
    return {
      step: 1,
      domain: '',
      added: '',
      app: initialApp ?? (apps.length === 1 ? (apps[0]?.name ?? '') : ''),
    }
  })
  const [touched, setTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  const [addError, setAddError] = useState('')
  const edit = useEditAppDomains(state.app)

  useEffect(() => {
    saveWizardState(state)
  }, [state])

  const domain = normalizeDomainInput(state.domain)
  const kind: DomainKind = classifyDomain(state.domain)
  const owner = claimedBy.get(domain)
  const problem = (() => {
    if (kind === 'empty') return 'empty'
    if (kind === 'invalid' || kind === 'wildcard') return kind
    if (owner !== undefined && domain !== state.added) {
      return owner === state.app ? 'claimedHere' : 'claimedOther'
    }
    return ''
  })()
  const showError = touched && problem !== ''

  function go(step: WizardStep) {
    setState((s) => ({ ...s, step }))
  }

  async function next() {
    setTouched(true)
    if (problem !== '' || state.app === '') return
    setAddError('')
    if (state.added !== domain) {
      setBusy(true)
      try {
        await edit.mutateAsync({ add: [domain] })
      } catch (err) {
        setAddError(err instanceof Error ? err.message : String(err))
        setBusy(false)
        return
      }
      setBusy(false)
    }
    setState((s) => ({ ...s, domain, added: domain, step: 2 }))
  }

  function restart() {
    clearWizardState()
    setTouched(false)
    setState({ step: 1, domain: '', added: '', app: state.app })
  }

  function finish() {
    clearWizardState()
    onClose()
  }

  return (
    <div className="space-y-4">
      <DialogHeader>
        <DialogTitle>
          {state.step === 1
            ? t('addWizard.step1.title')
            : state.step === 2
              ? t('addWizard.step2.title')
              : t('addWizard.step3.title')}
        </DialogTitle>
        <DialogDescription className="sr-only">
          {t('page.addDialog.description')}
        </DialogDescription>
      </DialogHeader>
      <Progress step={state.step} />

      {state.step === 1 ? (
        <form
          className="space-y-3"
          noValidate
          onSubmit={(e) => {
            e.preventDefault()
            void next()
          }}
        >
          <Field data-invalid={showError}>
            <FieldLabel htmlFor="add-wizard-domain">
              {t('addWizard.step1.domainLabel')}
            </FieldLabel>
            <Input
              id="add-wizard-domain"
              className="font-mono"
              autoFocus
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              value={state.domain}
              placeholder={t('addWizard.step1.domainPlaceholder')}
              aria-invalid={showError}
              onChange={(e) => {
                setState((s) => ({ ...s, domain: e.target.value }))
              }}
              onBlur={() => {
                setTouched(true)
              }}
            />
            {showError ? (
              <FieldError>
                {t(`addWizard.errors.${problem}`, { app: owner })}
              </FieldError>
            ) : kind === 'apex' || kind === 'subdomain' || kind === 'www' ? (
              <FieldHint>
                {t(
                  kind === 'apex'
                    ? 'addWizard.step1.kindApex'
                    : kind === 'www'
                      ? 'addWizard.step1.kindWww'
                      : 'addWizard.step1.kindSubdomain',
                )}
              </FieldHint>
            ) : null}
          </Field>
          <Field data-invalid={touched && state.app === ''}>
            <FieldLabel htmlFor="add-wizard-app">
              {t('addWizard.step1.appLabel')}
            </FieldLabel>
            <Select
              value={state.app || null}
              onValueChange={(v) => {
                setState((s) => ({ ...s, app: v ?? '' }))
              }}
            >
              <SelectTrigger id="add-wizard-app" className="w-full">
                <SelectValue
                  placeholder={t('addWizard.step1.appPlaceholder')}
                />
              </SelectTrigger>
              <SelectContent>
                {apps.map((a) => (
                  <SelectItem key={a.name} value={a.name}>
                    {a.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {touched && state.app === '' ? (
              <FieldError>{t('addWizard.errors.noApp')}</FieldError>
            ) : null}
          </Field>
          {addError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('addWizard.errors.addFailed', { message: addError })}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Button type="button" variant="link" size="sm" onClick={onAdvanced}>
              {t('addWizard.advanced')}
            </Button>
            <Button type="submit" disabled={busy}>
              {busy ? t('addWizard.step1.adding') : t('addWizard.next')}
            </Button>
          </div>
        </form>
      ) : null}

      {state.step === 2 ? (
        <PointStep
          app={state.app}
          domain={state.added}
          kind={classifyDomain(state.added)}
          onBack={() => {
            go(1)
          }}
          onContinue={() => {
            go(3)
          }}
        />
      ) : null}

      {state.step === 3 ? (
        <LiveStep
          app={state.app}
          domain={state.added}
          kind={classifyDomain(state.added)}
          onBack={() => {
            go(2)
          }}
          onAddAnother={restart}
          onFinishLater={finish}
        />
      ) : null}
    </div>
  )
}
