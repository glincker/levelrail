import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  PlugGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import { HelpLink } from '@/components/HelpLink'
import { cn } from '@/lib/utils'
import { useDialogParam } from '../kit/useUrlDialog'
import {
  useDisconnectNodeProvider,
  useSetNodeProviderCredential,
  useTestNodeProvider,
  type SetNodeProviderCredentialInput,
} from '../../queries/nodeProvision'
import { resolveMethod, supportedMethods } from '../../lib/nodeProviderMethods'
import type {
  NodeProviderMethod,
  NodeProviderResource,
  NodeProviderTestResult,
} from '../../types/nodeProvision'
import { NodeProviderPermissions } from './NodeProviderPermissions'

const METHOD_KEYS = {
  api_token: {
    title: 'nodeProviders.methods.api_token.title',
    description: 'nodeProviders.methods.api_token.description',
  },
  access_key: {
    title: 'nodeProviders.methods.access_key.title',
    description: 'nodeProviders.methods.access_key.description',
  },
  assume_role: {
    title: 'nodeProviders.methods.assume_role.title',
    description: 'nodeProviders.methods.assume_role.description',
  },
  ambient: {
    title: 'nodeProviders.methods.ambient.title',
    description: 'nodeProviders.methods.ambient.description',
  },
  workload_identity_federation: {
    title: 'nodeProviders.methods.workload_identity_federation.title',
    description:
      'nodeProviders.methods.workload_identity_federation.description',
  },
  service_account_json: {
    title: 'nodeProviders.methods.service_account_json.title',
    description: 'nodeProviders.methods.service_account_json.description',
  },
  manual: {
    title: 'nodeProviders.methods.manual.title',
    description: 'nodeProviders.methods.manual.description',
  },
} as const

const JSON_PLACEHOLDER: Record<string, string> = {
  azure:
    '{"tenant_id":"...","client_id":"...","client_secret":"...","subscription_id":"...","resource_group":"..."}',
  gcp: '{"type":"service_account","project_id":"...","private_key":"..."}',
}

const FEDERATION_PLACEHOLDER =
  '{"tenant_id":"...","client_id":"...","federated_token_file":"/var/run/secrets/azure/tokens/azure-identity-token","subscription_id":"...","resource_group":"..."}'

export function NodeProviderForm({
  provider,
  label,
  onSaved,
  onAddNode,
}: {
  provider: NodeProviderResource
  label: string
  onSaved: () => void
  onAddNode?: () => void
}) {
  const { t } = useTranslation('settings')
  const name = provider.provider
  const methods = supportedMethods(name, provider.methods)
  const [rawMethod, setRawMethod] = useDialogParam('method')
  const method = resolveMethod(rawMethod, methods, provider.method)
  const connected = provider.has_token

  const [token, setToken] = useState('')
  const [secretAccessKey, setSecretAccessKey] = useState('')
  const [sessionToken, setSessionToken] = useState('')
  const [region, setRegion] = useState('')
  const [roleARN, setRoleARN] = useState('')
  const [useAmbient, setUseAmbient] = useState(false)
  const [dcLabel, setDcLabel] = useState(provider.label ?? '')
  const [location, setLocation] = useState(provider.location ?? '')
  const [notes, setNotes] = useState(provider.notes ?? '')
  const [result, setResult] = useState<NodeProviderTestResult | null>(null)
  const [confirmDisconnect, setConfirmDisconnect] = useState(false)

  const connect = useSetNodeProviderCredential()
  const test = useTestNodeProvider()
  const disconnect = useDisconnectNodeProvider()
  const id = (field: string) => `node-provider-${field}-${name}`

  const needsKey = method === 'access_key'
  const needsRole = method === 'assume_role'
  const needsBase = needsRole && !useAmbient
  const isJSON =
    method === 'service_account_json' ||
    method === 'workload_identity_federation'
  const keyReady =
    token !== '' && (!(needsKey || needsBase) || secretAccessKey !== '')

  const canSubmit = (() => {
    switch (method) {
      case 'manual':
        return dcLabel.trim() !== ''
      case 'ambient':
        return true
      case 'assume_role':
        return roleARN.trim() !== '' && (useAmbient || keyReady)
      default:
        return keyReady
    }
  })()

  function body(): Omit<SetNodeProviderCredentialInput, 'provider'> {
    const base = { method, token }
    switch (method) {
      case 'manual':
        return {
          method,
          token: '',
          label: dcLabel.trim(),
          location: location.trim(),
          notes: notes.trim(),
        }
      case 'ambient':
        return { method, token: '', region: region || undefined }
      case 'access_key':
        return {
          ...base,
          secret_access_key: secretAccessKey,
          session_token: sessionToken || undefined,
          region: region || undefined,
        }
      case 'assume_role':
        return {
          method,
          token: useAmbient ? '' : token,
          secret_access_key: useAmbient ? undefined : secretAccessKey,
          session_token: sessionToken || undefined,
          region: region || undefined,
          role_arn: roleARN.trim(),
          use_ambient_credentials: useAmbient,
        }
      default:
        return base
    }
  }

  function runTest() {
    setResult(null)
    test.mutate({ provider: name, candidate: body() }, { onSuccess: setResult })
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    connect.mutate(
      { provider: name, verify: method !== 'manual', ...body() },
      {
        onSuccess: () => {
          toast.add({
            title: t('nodeProviders.saved', { provider: label }),
            type: 'success',
          })
          if (method === 'manual' && onAddNode) onAddNode()
          else onSaved()
        },
      },
    )
  }

  function runStoredTest() {
    setResult(null)
    test.mutate({ provider: name }, { onSuccess: setResult })
  }

  function runDisconnect() {
    disconnect.mutate(name, {
      onSuccess: () => {
        toast.add({
          title: t('nodeProviders.disconnected', { provider: label }),
          type: 'success',
        })
        onSaved()
      },
    })
  }

  const error = connect.isError
    ? connect.error.message
    : test.isError
      ? test.error.message
      : disconnect.isError
        ? disconnect.error.message
        : null

  return (
    <form onSubmit={submit} className="space-y-5">
      {methods.length > 1 ? (
        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">
            {t('nodeProviders.methodLegend')}
          </legend>
          <div role="radiogroup" className="grid gap-2">
            {methods.map((m: NodeProviderMethod) => (
              <button
                key={m}
                type="button"
                role="radio"
                aria-checked={m === method}
                onClick={() => {
                  setResult(null)
                  setRawMethod(m)
                }}
                className={cn(
                  'rounded-lg border p-2.5 text-left text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                  m === method
                    ? 'border-primary bg-primary/5'
                    : 'border-border hover:bg-muted/50',
                )}
              >
                <span className="block font-medium">
                  {t(METHOD_KEYS[m].title)}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(METHOD_KEYS[m].description)}
                </span>
              </button>
            ))}
          </div>
        </fieldset>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t(METHOD_KEYS[method].description)}
        </p>
      )}

      <FieldGroup className="gap-4">
        {method === 'manual' ? (
          <>
            <Field>
              <FieldLabel htmlFor={id('label')}>
                {t('nodeProviders.dcName')}
              </FieldLabel>
              <Input
                id={id('label')}
                value={dcLabel}
                onChange={(e) => setDcLabel(e.target.value)}
                maxLength={200}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={id('location')}>
                {t('nodeProviders.dcLocation')}
              </FieldLabel>
              <Input
                id={id('location')}
                value={location}
                onChange={(e) => setLocation(e.target.value)}
                maxLength={200}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={id('notes')}>
                {t('nodeProviders.dcNotes')}
              </FieldLabel>
              <Textarea
                id={id('notes')}
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                maxLength={500}
                rows={3}
              />
            </Field>
          </>
        ) : null}

        {method === 'assume_role' ? (
          <>
            <Field>
              <FieldLabel htmlFor={id('role')}>
                {t('nodeProviders.roleArnRequired')}
              </FieldLabel>
              <Input
                id={id('role')}
                value={roleARN}
                onChange={(e) => setRoleARN(e.target.value)}
                placeholder="arn:aws:iam::123456789012:role/example-provisioner"
                className="font-mono text-xs"
              />
              <FieldDescription>
                {t('nodeProviders.roleArnHelp')}
              </FieldDescription>
            </Field>
            <div className="flex items-start gap-2">
              <Checkbox
                id={id('ambient')}
                checked={useAmbient}
                onCheckedChange={(v) => setUseAmbient(v === true)}
              />
              <FieldLabel htmlFor={id('ambient')} className="font-normal">
                {t('nodeProviders.useAmbient')}
              </FieldLabel>
            </div>
          </>
        ) : null}

        {method === 'api_token' || isJSON || needsKey || needsBase ? (
          <Field>
            <FieldLabel htmlFor={id('token')}>
              {needsKey || needsBase
                ? t('nodeProviders.accessKeyId')
                : isJSON
                  ? t('nodeProviders.credentialJson')
                  : t('nodeProviders.apiToken')}
            </FieldLabel>
            {isJSON ? (
              <Textarea
                id={id('token')}
                autoComplete="off"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                rows={5}
                spellCheck={false}
                placeholder={
                  connected
                    ? t('nodeProviders.storedPlaceholder')
                    : method === 'workload_identity_federation'
                      ? FEDERATION_PLACEHOLDER
                      : (JSON_PLACEHOLDER[name] ?? '')
                }
                className="font-mono text-xs"
              />
            ) : (
              <Input
                id={id('token')}
                type="password"
                autoComplete="off"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder={
                  connected
                    ? t('nodeProviders.storedPlaceholder')
                    : needsKey || needsBase
                      ? 'AKIA...'
                      : ''
                }
              />
            )}
            <FieldDescription>
              {connected
                ? t('nodeProviders.storedReplace')
                : t('nodeProviders.neverEchoed')}
              {name === 'azure' ? (
                <>
                  {' '}
                  <HelpLink
                    path="/node-provisioning#azure-workload-identity-federation"
                    label={t('nodeProviders.azureFederationLink')}
                    variant="inline"
                  />
                </>
              ) : null}
            </FieldDescription>
          </Field>
        ) : null}

        {needsKey || needsBase ? (
          <>
            <Field>
              <FieldLabel htmlFor={id('secret')}>
                {t('nodeProviders.secretAccessKey')}
              </FieldLabel>
              <Input
                id={id('secret')}
                type="password"
                autoComplete="off"
                value={secretAccessKey}
                onChange={(e) => setSecretAccessKey(e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor={id('session')}>
                {t('nodeProviders.sessionToken')}
              </FieldLabel>
              <Input
                id={id('session')}
                type="password"
                autoComplete="off"
                value={sessionToken}
                onChange={(e) => setSessionToken(e.target.value)}
              />
            </Field>
          </>
        ) : null}

        {name === 'aws' && method !== 'manual' ? (
          <Field>
            <FieldLabel htmlFor={id('region')}>
              {t('nodeProviders.region')}
            </FieldLabel>
            <Input
              id={id('region')}
              value={region}
              onChange={(e) => setRegion(e.target.value)}
              placeholder="us-east-1"
              className="font-mono text-xs"
            />
          </Field>
        ) : null}
      </FieldGroup>

      <NodeProviderPermissions provider={name} method={method} />

      {result ? (
        <Alert variant={result.ok ? 'default' : 'destructive'}>
          {result.ok ? (
            <CircleCheckGlassIcon className="text-tone-success" size="1.2em" />
          ) : (
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
          )}
          <AlertDescription>{result.message}</AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Button type="submit" disabled={!canSubmit || connect.isPending}>
          {connect.isPending
            ? t('nodeProviders.saving')
            : method === 'manual'
              ? t('nodeProviders.saveManual')
              : t('nodeProviders.testAndConnect')}
        </Button>
        {method !== 'manual' ? (
          <Button
            type="button"
            variant="outline"
            disabled={!canSubmit || test.isPending}
            onClick={runTest}
          >
            <PlugGlassIcon size="1.2em" aria-hidden="true" />
            {test.isPending
              ? t('nodeProviders.testing')
              : t('nodeProviders.testConnection')}
          </Button>
        ) : null}
        {connected && method !== 'manual' ? (
          <Button
            type="button"
            variant="ghost"
            disabled={test.isPending}
            onClick={runStoredTest}
          >
            {t('nodeProviders.testStored')}
          </Button>
        ) : null}
        {connected && onAddNode ? (
          <Button type="button" variant="outline" onClick={onAddNode}>
            {t('nodeProviders.addNode')}
          </Button>
        ) : null}
        {connected ? (
          <Button
            type="button"
            variant="ghost"
            className="ml-auto text-destructive"
            disabled={disconnect.isPending}
            onClick={() =>
              confirmDisconnect ? runDisconnect() : setConfirmDisconnect(true)
            }
          >
            {confirmDisconnect
              ? t('nodeProviders.confirmDisconnect')
              : t('nodeProviders.disconnect')}
          </Button>
        ) : null}
      </div>
    </form>
  )
}
