import { useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowRightGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
  const confirmId = useId()
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
            <ArrowRightGlassIcon
              className="size-5 shrink-0 text-muted-foreground"
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
        <div
          role="alert"
          className="space-y-2 rounded-lg border border-tone-warning-border bg-tone-warning-soft p-2.5 text-sm"
        >
          <p className="flex items-start gap-2 font-medium">
            <TriangleAlertGlassIcon
              className="mt-0.5 size-5 shrink-0 text-tone-warning"
              aria-hidden="true"
            />
            {t('standalone.confirmTitle')}
          </p>
          <ul className="list-disc space-y-0.5 pl-10 text-muted-foreground">
            {reasons.map((r) => (
              <li key={r}>{r}</li>
            ))}
          </ul>
          <label htmlFor={confirmId} className="flex items-center gap-2 pl-6">
            <Checkbox
              id={confirmId}
              checked={confirmed}
              onCheckedChange={(c) => {
                setConfirmed(c)
              }}
            />
            {t('standalone.confirmCheck')}
          </label>
        </div>
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
