import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import { ociSessionCommand } from '../../lib/ociSession'
import { formatRelativeTime } from '../../lib/relativeTime'
import {
  useCreateOracleSessionAccount,
  useImportOracleSession,
} from '../../queries/cloudAccounts'
import type {
  OracleSessionCheck,
  OracleSessionInput,
} from '../../types/cloudAccounts'

// Format examples, not translatable copy.
const FORMAT = {
  region: 'eu-frankfurt-1',
  compartment: 'ocid1.compartment.oc1...',
} as const

// Browser session login for testing: files are pasted, checked offline, then stored sealed.
export function OracleSessionForm({
  label,
  onSaved,
}: Readonly<{ label: string; onSaved: () => void }>) {
  const { t } = useTranslation('nodes')
  const uid = useId()
  const [region, setRegion] = useState('')
  const [compartment, setCompartment] = useState('')
  const [profile, setProfile] = useState('')
  const [token, setToken] = useState('')
  const [key, setKey] = useState('')
  const [check, setCheck] = useState<OracleSessionCheck | null>(null)
  const importer = useImportOracleSession()
  const create = useCreateOracleSessionAccount()

  const canSubmit = token.trim() !== '' && key.trim() !== ''
  const body = (): OracleSessionInput => ({
    profile_config: profile.trim() || undefined,
    session_token: token.trim(),
    private_key_pem: key,
    region: region.trim() || undefined,
    compartment_ocid: compartment.trim() || undefined,
  })
  const error = create.isError
    ? create.error.message
    : importer.isError
      ? importer.error.message
      : null

  function runCheck() {
    setCheck(null)
    importer.mutate(body(), { onSuccess: setCheck })
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    create.mutate(
      { ...body(), tenancy_ocid: check?.tenancy_ocid, region: check?.region },
      {
        onSuccess: () => {
          setToken('')
          setKey('')
          toast.add({
            title: t('oracle.session.saved', { provider: label }),
            type: 'success',
          })
          onSaved()
        },
      },
    )
  }

  return (
    <form onSubmit={submit} className="space-y-4" autoComplete="off">
      <p className="text-sm text-muted-foreground">
        {t('oracle.session.intro')}
      </p>
      <Field>
        <FieldLabel htmlFor={`${uid}-region`}>
          {t('oracle.form.region')}
        </FieldLabel>
        <Input
          id={`${uid}-region`}
          value={region}
          onChange={(e) => setRegion(e.target.value)}
          placeholder={FORMAT.region}
          className="font-mono text-xs"
          spellCheck={false}
        />
      </Field>
      <Field>
        <FieldLabel>{t('oracle.session.command')}</FieldLabel>
        <code className="block overflow-x-auto rounded-md border border-border bg-muted/40 p-2 font-mono text-xs">
          {ociSessionCommand(region)}
        </code>
        <FieldDescription>{t('oracle.session.commandHelp')}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${uid}-compartment`}>
          {t('oracle.form.compartment')}
        </FieldLabel>
        <Input
          id={`${uid}-compartment`}
          value={compartment}
          onChange={(e) => setCompartment(e.target.value)}
          placeholder={FORMAT.compartment}
          className="font-mono text-xs"
          spellCheck={false}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${uid}-profile`}>
          {t('oracle.session.profile')}
        </FieldLabel>
        <Textarea
          id={`${uid}-profile`}
          value={profile}
          onChange={(e) => setProfile(e.target.value)}
          rows={3}
          spellCheck={false}
          placeholder="tenancy=ocid1.tenancy.oc1...&#10;region=eu-frankfurt-1"
          className="font-mono text-xs"
        />
        <FieldDescription>{t('oracle.session.profileHelp')}</FieldDescription>
      </Field>
      <Field>
        <FieldLabel htmlFor={`${uid}-token`}>
          {t('oracle.session.token')}
        </FieldLabel>
        <Textarea
          id={`${uid}-token`}
          autoComplete="off"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          rows={3}
          spellCheck={false}
          className="font-mono text-xs"
        />
      </Field>
      <Field>
        <FieldLabel htmlFor={`${uid}-key`}>
          {t('oracle.session.key')}
        </FieldLabel>
        <Textarea
          id={`${uid}-key`}
          autoComplete="off"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          rows={4}
          spellCheck={false}
          placeholder="-----BEGIN PRIVATE KEY-----"
          className="font-mono text-xs"
        />
        <FieldDescription>{t('oracle.session.keyHelp')}</FieldDescription>
      </Field>

      {check ? (
        <Alert variant={check.needs_relogin ? 'destructive' : 'default'}>
          {check.needs_relogin ? (
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
          ) : (
            <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
          )}
          <AlertDescription>
            {t('oracle.session.expires', {
              when: formatRelativeTime(check.expires_at),
            })}
          </AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" disabled={!canSubmit || create.isPending}>
          {create.isPending
            ? t('oracle.form.saving')
            : t('oracle.session.connect')}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!canSubmit || importer.isPending}
          onClick={runCheck}
        >
          {importer.isPending
            ? t('oracle.form.testing')
            : t('oracle.session.check')}
        </Button>
      </div>
    </form>
  )
}
