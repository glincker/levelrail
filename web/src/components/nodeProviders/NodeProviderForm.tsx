import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import { HelpLink } from '@/components/HelpLink'
import { useSetNodeProviderCredential } from '../../queries/nodeProvision'
import type { NodeProviderResource } from '../../types/nodeProvision'

const JSON_PLACEHOLDER: Record<string, string> = {
  azure:
    '{"tenant_id":"...","client_id":"...","client_secret":"...","subscription_id":"...","resource_group":"..."}',
  gcp: '{"type":"service_account","project_id":"...","private_key":"..."}',
}

const JSON_PROVIDERS = new Set(['azure', 'gcp'])

export function NodeProviderForm({
  provider,
  label,
  onSaved,
}: {
  provider: NodeProviderResource
  label: string
  onSaved: () => void
}) {
  const { t } = useTranslation('settings')
  const isAWS = provider.provider === 'aws'
  const isJSON = JSON_PROVIDERS.has(provider.provider)
  const [token, setToken] = useState('')
  const [secretAccessKey, setSecretAccessKey] = useState('')
  const [sessionToken, setSessionToken] = useState('')
  const [region, setRegion] = useState('')
  const [roleARN, setRoleARN] = useState('')
  const [useAmbient, setUseAmbient] = useState(false)
  const setCredential = useSetNodeProviderCredential()
  const id = (name: string) => `node-provider-${name}-${provider.provider}`

  const canSubmit = isAWS
    ? useAmbient || (token !== '' && secretAccessKey !== '')
    : token !== ''

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!canSubmit) return
    setCredential.mutate(
      {
        provider: provider.provider,
        token,
        ...(isAWS
          ? {
              secret_access_key: secretAccessKey,
              session_token: sessionToken || undefined,
              region: region || undefined,
              role_arn: roleARN || undefined,
              use_ambient_credentials: useAmbient,
            }
          : {}),
      },
      {
        onSuccess: () => {
          toast.add({
            title: t('nodeProviders.saved', { provider: label }),
            type: 'success',
          })
          onSaved()
        },
      },
    )
  }

  const tokenLabel = isAWS
    ? t('nodeProviders.accessKeyId')
    : isJSON
      ? provider.provider === 'azure'
        ? t('nodeProviders.azureCredential')
        : t('nodeProviders.gcpCredential')
      : t('nodeProviders.apiToken')

  return (
    <form onSubmit={handleSubmit} className="space-y-5 pt-4">
      <FieldGroup className="gap-4">
        {isAWS ? (
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
        ) : null}
        {!isAWS || !useAmbient ? (
          <Field>
            <FieldLabel htmlFor={id('token')}>{tokenLabel}</FieldLabel>
            <Input
              id={id('token')}
              type="password"
              autoComplete="off"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={
                provider.has_token
                  ? '••••••••••••'
                  : (JSON_PLACEHOLDER[provider.provider] ??
                    (isAWS ? 'AKIA...' : ''))
              }
              className={isJSON ? 'font-mono text-xs' : undefined}
            />
            <FieldDescription>
              {provider.has_token
                ? t('nodeProviders.storedReplace')
                : t('nodeProviders.neverEchoed')}
              {provider.provider === 'azure' ? (
                <>
                  {' '}
                  {t('nodeProviders.azureFederation')}{' '}
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
        {isAWS && !useAmbient ? (
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
        ) : null}
        {isAWS ? (
          <>
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
            <Field>
              <FieldLabel htmlFor={id('role')}>
                {t('nodeProviders.roleArn')}
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
          </>
        ) : null}
      </FieldGroup>
      {setCredential.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{setCredential.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button type="submit" disabled={!canSubmit || setCredential.isPending}>
        {setCredential.isPending
          ? t('nodeProviders.saving')
          : t('nodeProviders.save')}
      </Button>
    </form>
  )
}
