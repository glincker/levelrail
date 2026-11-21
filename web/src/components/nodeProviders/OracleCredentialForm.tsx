import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronRightGlassIcon,
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
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { OCI_RE, parseOciConfig, shortOcid } from '@/lib/ociConfig'
import { OracleKeyWizard } from './OracleKeyWizard'
import { OracleSessionForm } from './OracleSessionForm'
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

const CONFIG_PLACEHOLDER = [
  '[DEFAULT]',
  'user=ocid1.user.oc1...',
  'fingerprint=aa:bb:cc:...',
  'tenancy=ocid1.tenancy.oc1...',
  'region=us-phoenix-1',
  'key_file=~/.oci/oci_api_key.pem',
].join('\n')

// Format examples, not translatable copy.
const FORMAT = {
  tenancy: 'ocid1.tenancy.oc1...',
  user: 'ocid1.user.oc1...',
  fingerprint: 'aa:bb:cc:...',
  region: 'eu-frankfurt-1',
  compartment: 'ocid1.compartment.oc1...',
  subnet: 'ocid1.subnet.oc1...',
} as const

function bad<K extends string>(value: string, re: RegExp, key: K): K | null {
  return value.trim() !== '' && !re.test(value.trim()) ? key : null
}

// The private key is held only in this component state and cleared the moment
// a save succeeds; the API never returns it, so there is nothing to prefill.
export function OracleCredentialForm({
  provider,
  label,
  onSaved,
  requireSubnet = true,
}: {
  provider: NodeProviderResource
  label: string
  onSaved: () => void
  // The node provider endpoint needs a subnet to create servers.
  requireSubnet?: boolean
}) {
  const { t } = useTranslation('nodes')
  const connected = provider.has_token
  const [configText, setConfigText] = useState('')
  const [keyFile, setKeyFile] = useState('')
  const [editing, setEditing] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
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
  const [mode, setMode] = useState<'api_key' | 'session'>('api_key')
  const fileRef = useRef<HTMLInputElement>(null)
  const configFileRef = useRef<HTMLInputElement>(null)

  const connect = useSetNodeProviderCredential()
  const test = useTestNodeProvider()
  const disconnect = useDisconnectNodeProvider()
  const id = (field: string) => `node-provider-oracle-${field}`

  // The root compartment is the tenancy itself, the usual first choice.
  const effectiveCompartment = compartmentTouched ? compartment : tenancy
  const fields = { tenancy, user, fingerprint, region } as const
  const missing = (Object.keys(fields) as (keyof typeof fields)[]).filter(
    (k) => fields[k].trim() === '',
  )
  const allFour = missing.length === 0
  const errs = {
    tenancy: bad(tenancy, OCI_RE.tenancy, 'oracle.form.errors.tenancy'),
    user: bad(user, OCI_RE.user, 'oracle.form.errors.user'),
    fingerprint: bad(
      fingerprint,
      OCI_RE.fingerprint,
      'oracle.form.errors.fingerprint',
    ),
    region: bad(region, OCI_RE.region, 'oracle.form.errors.region'),
    compartment: bad(
      effectiveCompartment,
      OCI_RE.compartment,
      'oracle.form.errors.compartment',
    ),
    subnet: bad(subnet, OCI_RE.subnet, 'oracle.form.errors.subnet'),
    key:
      privateKey.trim() !== '' && !OCI_RE.key.test(privateKey)
        ? ('oracle.form.errors.key' as const)
        : null,
  }
  const hasErr = Object.values(errs).some(Boolean)
  const detected = allFour && !editing && !errs.tenancy && !errs.region
  const showFields = editing || (configText.trim() !== '' && !allFour)
  const subnetMissing = requireSubnet && subnet.trim() === ''
  const baseReady =
    allFour &&
    privateKey.trim() !== '' &&
    effectiveCompartment.trim() !== '' &&
    !hasErr
  const canSubmit = baseReady && !subnetMissing

  function applyConfig(text: string) {
    setConfigText(text)
    const c = parseOciConfig(text)
    // Only overwrite with what the config actually carries.
    if (c.tenancy) setTenancy(c.tenancy)
    if (c.user) setUser(c.user)
    if (c.fingerprint) setFingerprint(c.fingerprint)
    if (c.region) setRegion(c.region)
    setKeyFile(c.keyFile)
    setEditing(false)
  }

  async function loadConfigFile(file: File | undefined) {
    if (!file) return
    applyConfig(await file.text())
    if (configFileRef.current) configFileRef.current.value = ''
  }

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

  const modeToggle = (
    <div
      role="group"
      aria-label={t('oracle.session.modeLabel')}
      className="flex flex-wrap gap-2"
    >
      <Button
        type="button"
        size="sm"
        variant={mode === 'api_key' ? 'default' : 'outline'}
        aria-pressed={mode === 'api_key'}
        onClick={() => setMode('api_key')}
      >
        {t('oracle.session.modeKey')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant={mode === 'session' ? 'default' : 'outline'}
        aria-pressed={mode === 'session'}
        onClick={() => setMode('session')}
      >
        {t('oracle.session.modeSession')}
      </Button>
    </div>
  )

  if (mode === 'session') {
    return (
      <div className="space-y-4">
        {modeToggle}
        <OracleSessionForm label={label} onSaved={onSaved} />
      </div>
    )
  }

  return (
    <form onSubmit={submit} className="space-y-3" autoComplete="off">
      {modeToggle}
      <ol className="list-inside list-decimal space-y-0.5 rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
        <li className="font-medium text-foreground">
          {t('oracle.form.howTitle')}
        </li>
        <li>{t('oracle.form.how1')}</li>
        <li>{t('oracle.form.how2')}</li>
        <li>{t('oracle.form.how3')}</li>
      </ol>
      <div className="grid gap-3 md:grid-cols-2">
        <Field>
          <div className="flex items-center justify-between gap-2">
            <FieldLabel htmlFor={id('config')}>
              {t('oracle.form.configTitle')}
            </FieldLabel>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => configFileRef.current?.click()}
            >
              {t('oracle.form.configFile')}
            </Button>
            <input
              ref={configFileRef}
              type="file"
              className="sr-only"
              tabIndex={-1}
              aria-label={t('oracle.form.configFile')}
              onChange={(e) => void loadConfigFile(e.target.files?.[0])}
            />
          </div>
          <Textarea
            id={id('config')}
            value={configText}
            onChange={(e) => applyConfig(e.target.value)}
            rows={5}
            spellCheck={false}
            placeholder={CONFIG_PLACEHOLDER}
            className="font-mono text-xs"
          />
          {!showFields && configText.trim() === '' ? (
            <FieldDescription>{t('oracle.form.configHelp')}</FieldDescription>
          ) : null}
        </Field>
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
            rows={5}
            spellCheck={false}
            aria-invalid={errs.key ? true : undefined}
            placeholder={
              connected
                ? t('oracle.form.keyStored')
                : '-----BEGIN PRIVATE KEY-----'
            }
            className="font-mono text-xs"
          />
          <FieldDescription>
            {errs.key
              ? t(errs.key)
              : keyFile
                ? t('oracle.form.keyFromConfig', { path: keyFile })
                : connected
                  ? t('oracle.form.keyStoredHelp')
                  : t('oracle.form.keyHelp')}
          </FieldDescription>
        </Field>
      </div>

      {showFields ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {(
            [
              ['tenancy', tenancy, setTenancy],
              ['user', user, setUser],
              ['fingerprint', fingerprint, setFingerprint],
              ['region', region, setRegion],
            ] as const
          ).map(([key, value, set]) => (
            <Field key={key}>
              <FieldLabel htmlFor={id(key)}>
                {t(`oracle.form.${key}`)}
              </FieldLabel>
              <Input
                id={id(key)}
                value={value}
                onChange={(e) => set(e.target.value)}
                placeholder={FORMAT[key]}
                aria-invalid={errs[key] ? true : undefined}
                aria-describedby={errs[key] ? id(`${key}-err`) : undefined}
                className="font-mono text-xs"
                spellCheck={false}
              />
              {errs[key] ? (
                <FieldDescription
                  id={id(`${key}-err`)}
                  className="text-destructive"
                >
                  {t(errs[key])}
                </FieldDescription>
              ) : null}
            </Field>
          ))}
        </div>
      ) : null}

      {detected ? (
        <div
          role="status"
          className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2 text-sm"
        >
          <CircleCheckGlassIcon
            className="text-tone-success"
            size="1.4em"
            aria-hidden="true"
          />
          <span className="text-tone-success">
            {t('oracle.form.detected', {
              tenancy: shortOcid(tenancy),
              region: region.trim(),
            })}
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="ml-auto"
            onClick={() => setEditing(true)}
          >
            {t('oracle.form.edit')}
          </Button>
        </div>
      ) : editing || missing.length > 0 ? (
        <p className="text-xs text-muted-foreground">
          {configText.trim() !== '' && missing.length > 0
            ? t('oracle.form.missing', {
                fields: missing.map((f) => t(`oracle.form.${f}`)).join(', '),
              })
            : t('oracle.form.manualHelp')}
          {editing && allFour ? (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="ml-2"
              onClick={() => setEditing(false)}
            >
              {t('oracle.form.done')}
            </Button>
          ) : null}
        </p>
      ) : null}

      <Collapsible
        open={advancedOpen}
        onOpenChange={setAdvancedOpen}
        className="rounded-lg border border-border bg-muted/30 p-3"
      >
        <CollapsibleTrigger className="flex items-center gap-2 text-sm font-medium">
          <ChevronRightGlassIcon
            size="1.4em"
            aria-hidden="true"
            className={
              advancedOpen
                ? 'rotate-90 transition-transform motion-reduce:transition-none'
                : 'transition-transform motion-reduce:transition-none'
            }
          />
          {t('oracle.form.advanced')}
          <span className="text-xs font-normal text-muted-foreground">
            {requireSubnet
              ? t('oracle.form.advancedHintServers')
              : t('oracle.form.advancedHint')}
          </span>
        </CollapsibleTrigger>
        <CollapsiblePanel className="space-y-3 pt-3">
          <div className="grid gap-3 sm:grid-cols-2">
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
                aria-invalid={errs.compartment ? true : undefined}
                className="font-mono text-xs"
                spellCheck={false}
              />
              <FieldDescription>
                {errs.compartment
                  ? t(errs.compartment)
                  : t('oracle.form.compartmentHelp')}
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
                aria-invalid={errs.subnet ? true : undefined}
                className="font-mono text-xs"
                spellCheck={false}
              />
              <FieldDescription>
                {errs.subnet
                  ? t(errs.subnet)
                  : requireSubnet
                    ? t('oracle.form.subnetHelp')
                    : t('oracle.form.subnetOptional')}
              </FieldDescription>
            </Field>
          </div>
          <OracleKeyWizard
            connected={connected}
            onUseKey={(pem, fp) => {
              setPrivateKey(pem)
              setFingerprint(fp)
            }}
          />
        </CollapsiblePanel>
      </Collapsible>

      <p className="text-xs text-muted-foreground">
        <span className="font-medium text-foreground">
          {t('oracle.form.permissionsTitle')}:
        </span>{' '}
        {t('oracle.form.permissions')}
      </p>

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

      {baseReady && subnetMissing ? (
        <p className="text-xs text-muted-foreground">
          {t('oracle.form.subnetNeeded')}
        </p>
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
