import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowRightIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import {
  DEFAULT_ADVANCED,
  EMPTY_TARGET,
  advancedFrom,
  buildStandaloneInput,
  draftFromTarget,
  targetString,
  type AdvancedDraft,
  type TargetDraft,
} from '../../lib/standaloneDomain'
import {
  StandaloneConfirmationError,
  useCreateStandaloneDomain,
  useUpdateStandaloneDomain,
  useValidateStandalone,
  type StandaloneDomain,
  type StandaloneDomainInput,
} from '../../queries/standaloneDomains'
import { StandaloneAdvancedOptions } from './StandaloneAdvancedOptions'
import { StandaloneTargetPicker } from './StandaloneTargetPicker'

const VALIDATE_DELAY_MS = 400

export function StandaloneDomainForm({
  initial,
  onSaved,
}: {
  initial?: StandaloneDomain
  onSaved: (d: StandaloneDomain) => void
}) {
  const { t } = useTranslation('domains')
  const editing = initial !== undefined
  const [domain, setDomain] = useState(initial?.domain ?? '')
  const [draft, setDraft] = useState<TargetDraft>(
    initial ? draftFromTarget(initial.target) : EMPTY_TARGET,
  )
  const [adv, setAdv] = useState<AdvancedDraft>(
    initial ? advancedFrom(initial) : DEFAULT_ADVANCED,
  )
  const [confirmed, setConfirmed] = useState(false)
  const [lateReasons, setLateReasons] = useState<string[]>([])
  const create = useCreateStandaloneDomain()
  const update = useUpdateStandaloneDomain(initial?.domain ?? '')
  const mutation = editing ? update : create

  const inputKey = JSON.stringify(buildStandaloneInput(domain, draft, adv))
  const debouncedKey = useDebouncedValue(inputKey, VALIDATE_DELAY_MS)
  const debounced = useMemo(
    () => JSON.parse(debouncedKey) as StandaloneDomainInput,
    [debouncedKey],
  )
  const validation = useValidateStandalone(debounced, Boolean(debounced.target))
  const v = validation.data
  const reasons = v?.assessment.needs_confirmation?.length
    ? v.assessment.needs_confirmation
    : lateReasons
  const domainError = editing ? undefined : v?.domain_error
  const targetError = v?.target_error
  const target = targetString(draft)
  const ready = Boolean(
    domain.trim() &&
    target &&
    !domainError &&
    !targetError &&
    (reasons.length === 0 || confirmed),
  )

  function submit() {
    const input = {
      ...buildStandaloneInput(domain, draft, adv),
      confirm: confirmed,
    }
    mutation.mutate(input, {
      onSuccess: onSaved,
      onError: (err) => {
        if (err instanceof StandaloneConfirmationError) {
          setLateReasons(err.reasons)
          setConfirmed(false)
        }
      },
    })
  }

  return (
    <div className="space-y-4">
      <Field>
        <FieldLabel htmlFor="standalone-domain">
          {t('standalone.domain')}
        </FieldLabel>
        <Input
          id="standalone-domain"
          className="font-mono"
          value={domain}
          disabled={editing}
          placeholder={t('standalone.domainPlaceholder')}
          onChange={(e) => {
            setDomain(e.target.value.trim())
          }}
        />
        {domainError && domain ? <FieldError>{domainError}</FieldError> : null}
      </Field>

      <Field>
        <FieldLabel>{t('standalone.target')}</FieldLabel>
        <StandaloneTargetPicker
          draft={draft}
          onChange={(next) => {
            setDraft(next)
            setConfirmed(false)
            setLateReasons([])
          }}
        />
        {targetError && target ? <FieldError>{targetError}</FieldError> : null}
      </Field>

      <div
        className="rounded-md border border-border bg-muted/40 px-3 py-2"
        aria-live="polite"
      >
        <p className="text-xs text-muted-foreground">
          {t('standalone.preview')}
        </p>
        {target ? (
          <p className="flex min-w-0 flex-wrap items-center gap-1.5 font-mono text-sm">
            <span className="break-all">
              https://{domain || t('standalone.domainPlaceholder')}
            </span>
            <ArrowRightIcon
              className="size-3.5 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
            <span className="break-all">{v?.target_url ?? target}</span>
          </p>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('standalone.previewEmpty')}
          </p>
        )}
        {validation.isFetching ? (
          <p className="text-xs text-muted-foreground">
            {t('standalone.checking')}
          </p>
        ) : null}
      </div>

      <StandaloneAdvancedOptions
        value={adv}
        urlTarget={draft.mode === 'url'}
        onChange={setAdv}
      />

      {reasons.length > 0 ? (
        <Alert>
          <WarningIcon className="text-amber-600" aria-hidden="true" />
          <AlertTitle>{t('standalone.confirmTitle')}</AlertTitle>
          <AlertDescription>
            <ul className="list-disc space-y-0.5 pl-4">
              {reasons.map((r) => (
                <li key={r}>{r}</li>
              ))}
            </ul>
            <label className="mt-2 flex items-center gap-2 text-foreground">
              <Checkbox
                checked={confirmed}
                onCheckedChange={(c) => {
                  setConfirmed(c)
                }}
              />
              {t('standalone.confirmCheck')}
            </label>
          </AlertDescription>
        </Alert>
      ) : null}

      {mutation.isError &&
      !(mutation.error instanceof StandaloneConfirmationError) ? (
        <Alert variant="destructive">
          <AlertDescription>{mutation.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex justify-end">
        <Button
          type="button"
          disabled={!ready || mutation.isPending}
          onClick={submit}
        >
          {mutation.isPending
            ? t('standalone.saving')
            : editing
              ? t('standalone.save')
              : t('standalone.submit')}
        </Button>
      </div>
    </div>
  )
}
