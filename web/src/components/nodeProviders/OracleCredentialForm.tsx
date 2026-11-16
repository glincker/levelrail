import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  PlugGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import {
  useDisconnectNodeProvider,
  useSetNodeProviderCredential,
  useTestNodeProvider,
  type SetNodeProviderCredentialInput,
} from '../../queries/nodeProvision'
import type {
  NodeProviderResource,
  NodeProviderTestResult,
} from '../../types/nodeProvision'

// Format examples, not translatable copy.
const FORMAT = {
  tenancy: 'ocid1.tenancy.oc1...',
  user: 'ocid1.user.oc1...',
  fingerprint: 'aa:bb:cc:...',
  region: 'eu-frankfurt-1',
  compartment: 'ocid1.compartment.oc1...',
  subnet: 'ocid1.subnet.oc1...',
} as const

// The private key is held only in this component state and cleared the moment
// a save succeeds; the API never returns it, so there is nothing to prefill.
export function OracleCredentialForm({
  provider,
  label,
  onSaved,
}: {
  provider: NodeProviderResource
  label: string
  onSaved: () => void
}) {
  const { t } = useTranslation('nodes')
  const connected = provider.has_token
  const [tenancy, setTenancy] = useState('')
  const [user, setUser] = useState('')
  const [fingerprint, setFingerprint] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [region, setRegion] = useState('')
  const [compartment, setCompartment] = useState('')
  const [compartmentTouched, setCompartmentTouched] = useState(false)
  const [subnet, setSubnet] = useState('')
  const [result, setResult] = useState<NodeProviderTestResult | null>(null)
  const [confirmDisconnect, setConfirmDisconnect] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  const connect = useSetNodeProviderCredential()
  const test = useTestNodeProvider()
  const disconnect = useDisconnectNodeProvider()
  const id = (field: string) => `node-provider-oracle-${field}`

  // The root compartment is the tenancy itself, the usual first choice.
  const effectiveCompartment = compartmentTouched ? compartment : tenancy
  const canSubmit =
    [tenancy, user, fingerprint, privateKey, region, subnet].every(
      (v) => v.trim() !== '',
    ) && effectiveCompartment.trim() !== ''

  function body(): Omit<SetNodeProviderCredentialInput, 'provider'> {
    return {
      method: 'api_key',
      token: '',
      tenancy_ocid: tenancy.trim(),
      user_ocid: user.trim(),
      fingerprint: fingerprint.trim(),
      private_key_pem: privateKey,
      region: region.trim(),
      compartment_ocid: effectiveCompartment.trim(),
      subnet_ocid: subnet.trim(),
    }
  }

  function runTest() {
    setResult(null)
    test.mutate(
      { provider: 'oracle', candidate: body() },
      { onSuccess: setResult },
    )
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    connect.mutate(
      { provider: 'oracle', verify: true, ...body() },
      {
        onSuccess: () => {
          setPrivateKey('')
          toast.add({
            title: t('oracle.form.saved', { provider: label }),
            type: 'success',
          })
          onSaved()
        },
      },
    )
  }

  async function loadKeyFile(file: File | undefined) {
    if (!file) return
    setPrivateKey(await file.text())
    if (fileRef.current) fileRef.current.value = ''
  }

  const error = connect.isError
    ? connect.error.message
    : test.isError
      ? test.error.message
      : disconnect.isError
        ? disconnect.error.message
        : null

  return (
    <form onSubmit={submit} className="space-y-4" autoComplete="off">
      <p className="text-sm text-muted-foreground">{t('oracle.form.intro')}</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor={id('tenancy')}>
            {t('oracle.form.tenancy')}
          </FieldLabel>
          <Input
            id={id('tenancy')}
            value={tenancy}
            onChange={(e) => setTenancy(e.target.value)}
            placeholder={FORMAT.tenancy}
            className="font-mono text-xs"
            spellCheck={false}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id('user')}>{t('oracle.form.user')}</FieldLabel>
          <Input
            id={id('user')}
            value={user}
            onChange={(e) => setUser(e.target.value)}
            placeholder={FORMAT.user}
            className="font-mono text-xs"
            spellCheck={false}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id('fingerprint')}>
            {t('oracle.form.fingerprint')}
          </FieldLabel>
          <Input
            id={id('fingerprint')}
            value={fingerprint}
            onChange={(e) => setFingerprint(e.target.value)}
            placeholder={FORMAT.fingerprint}
            className="font-mono text-xs"
            spellCheck={false}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id('region')}>
            {t('oracle.form.region')}
          </FieldLabel>
          <Input
            id={id('region')}
            value={region}
            onChange={(e) => setRegion(e.target.value)}
            placeholder={FORMAT.region}
            className="font-mono text-xs"
            spellCheck={false}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id('compartment')}>
            {t('oracle.form.compartment')}
          </FieldLabel>
          <Input
            id={id('compartment')}
            value={effectiveCompartment}
            onChange={(e) => {
              setCompartmentTouched(true)
              setCompartment(e.target.value)
            }}
            placeholder={FORMAT.compartment}
            className="font-mono text-xs"
            spellCheck={false}
          />
          <FieldDescription>
            {t('oracle.form.compartmentHelp')}
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={id('subnet')}>
            {t('oracle.form.subnet')}
          </FieldLabel>
          <Input
            id={id('subnet')}
            value={subnet}
            onChange={(e) => setSubnet(e.target.value)}
            placeholder={FORMAT.subnet}
            className="font-mono text-xs"
            spellCheck={false}
          />
        </Field>
      </div>
      <Field>
        <div className="flex items-center justify-between gap-2">
          <FieldLabel htmlFor={id('key')}>{t('oracle.form.key')}</FieldLabel>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => fileRef.current?.click()}
          >
            {t('oracle.form.keyFile')}
          </Button>
          <input
            ref={fileRef}
            type="file"
            accept=".pem,.key,text/plain"
            className="sr-only"
            tabIndex={-1}
            aria-label={t('oracle.form.keyFile')}
            onChange={(e) => void loadKeyFile(e.target.files?.[0])}
          />
        </div>
        <Textarea
          id={id('key')}
          autoComplete="off"
          value={privateKey}
          onChange={(e) => setPrivateKey(e.target.value)}
          rows={4}
          spellCheck={false}
          placeholder={
            connected
              ? t('oracle.form.keyStored')
              : '-----BEGIN PRIVATE KEY-----'
          }
          className="font-mono text-xs"
        />
        <FieldDescription>
          {connected
            ? t('oracle.form.keyStoredHelp')
            : t('oracle.form.keyHelp')}
        </FieldDescription>
      </Field>

      <section
        aria-label={t('oracle.form.permissionsTitle')}
        className="space-y-1 rounded-lg border border-border bg-muted/30 p-3 text-xs"
      >
        <h3 className="text-sm font-medium">
          {t('oracle.form.permissionsTitle')}
        </h3>
        <p className="text-muted-foreground">{t('oracle.form.permissions')}</p>
      </section>

      {result ? (
        <Alert variant={result.ok ? 'default' : 'destructive'}>
          {result.ok ? (
            <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
          ) : (
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
            />
          )}
          <AlertDescription>{result.message}</AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" disabled={!canSubmit || connect.isPending}>
          {connect.isPending
            ? t('oracle.form.saving')
            : t('oracle.form.testAndConnect')}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!canSubmit || test.isPending}
          onClick={runTest}
        >
          <PlugGlassIcon size="1.4em" aria-hidden="true" />
          {test.isPending ? t('oracle.form.testing') : t('oracle.form.test')}
        </Button>
        {connected ? (
          <Button
            type="button"
            variant="ghost"
            disabled={test.isPending}
            onClick={() => {
              setResult(null)
              test.mutate({ provider: 'oracle' }, { onSuccess: setResult })
            }}
          >
            {t('oracle.form.testStored')}
          </Button>
        ) : null}
        {connected ? (
          <Button
            type="button"
            variant="ghost"
            className="ml-auto text-destructive"
            disabled={disconnect.isPending}
            onClick={() => {
              if (!confirmDisconnect) {
                setConfirmDisconnect(true)
                return
              }
              disconnect.mutate('oracle', {
                onSuccess: () => {
                  toast.add({
                    title: t('oracle.form.disconnected', { provider: label }),
                    type: 'success',
                  })
                  onSaved()
                },
              })
            }}
          >
            {confirmDisconnect
              ? t('oracle.form.confirmDisconnect')
              : t('oracle.form.disconnect')}
          </Button>
        ) : null}
      </div>
    </form>
  )
}
