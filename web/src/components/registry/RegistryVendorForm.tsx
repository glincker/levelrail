import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  PlugGlassIcon,
  ShieldCheckGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SecretInput } from '@/components/ui/secret-input'
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateRegistryCredential } from '../../queries/registryCredentials'
import { useProbeRegistryCredential } from '../../queries/registryVendors'
import type {
  ConnectionTestResult,
  RegistryVendor,
} from '../../types/registryVendor'

const AUTH_KEYS = {
  dockerhub: 'registryVendors.auth.dockerhub',
  ghcr: 'registryVendors.auth.ghcr',
  gitlab: 'registryVendors.auth.gitlab',
  ecr: 'registryVendors.auth.ecr',
  gar: 'registryVendors.auth.gar',
  acr: 'registryVendors.auth.acr',
  harbor: 'registryVendors.auth.harbor',
  quay: 'registryVendors.auth.quay',
  jfrog: 'registryVendors.auth.jfrog',
  nexus: 'registryVendors.auth.nexus',
  generic: 'registryVendors.auth.generic',
} as const

const SCOPE_KEYS = {
  dockerhub: 'registryVendors.scopes.dockerhub',
  ghcr: 'registryVendors.scopes.ghcr',
  gitlab: 'registryVendors.scopes.gitlab',
  ecr: 'registryVendors.scopes.ecr',
  gar: 'registryVendors.scopes.gar',
  acr: 'registryVendors.scopes.acr',
  harbor: 'registryVendors.scopes.harbor',
  quay: 'registryVendors.scopes.quay',
  jfrog: 'registryVendors.scopes.jfrog',
  nexus: 'registryVendors.scopes.nexus',
  generic: 'registryVendors.scopes.generic',
} as const

type KnownVendor = keyof typeof AUTH_KEYS

function knownVendor(id: string): KnownVendor {
  return id in AUTH_KEYS ? (id as KnownVendor) : 'generic'
}

// Expiry the form pre-fills for vendors whose passwords are short lived,
// so the existing expiry reminder fires before a pull fails.
function defaultExpiry(hours: number | undefined): string {
  if (!hours) return ''
  return new Date(Date.now() + hours * 3_600_000).toISOString().slice(0, 10)
}

export function RegistryVendorForm({
  vendor,
  onDone,
}: {
  vendor: RegistryVendor
  onDone: () => void
}) {
  const { t } = useTranslation('settings')
  const key = knownVendor(vendor.id)
  const [name, setName] = useState(vendor.id)
  const [host, setHost] = useState(vendor.default_host ?? '')
  const [username, setUsername] = useState(vendor.fixed_username ?? '')
  const [password, setPassword] = useState('')
  const [expiresAt, setExpiresAt] = useState(
    defaultExpiry(vendor.token_lifetime_hours),
  )
  const [result, setResult] = useState<ConnectionTestResult | null>(null)
  const create = useCreateRegistryCredential()
  const probe = useProbeRegistryCredential()
  const id = (field: string) => `registry-vendor-${field}-${vendor.id}`

  const ready =
    name.trim() !== '' &&
    host.trim() !== '' &&
    username.trim() !== '' &&
    password !== ''

  function runTest() {
    setResult(null)
    probe.mutate(
      {
        registry_host: host.trim(),
        username: username.trim(),
        password,
      },
      { onSuccess: setResult },
    )
  }

  function save(e: React.FormEvent) {
    e.preventDefault()
    if (!ready) return
    create.mutate(
      {
        name: name.trim(),
        registry_host: host.trim(),
        username: username.trim(),
        password,
        expires_at: expiresAt ? new Date(expiresAt).toISOString() : undefined,
      },
      {
        onSuccess: (created) => {
          toast.add({
            title: t('registryVendors.saved', { name: created.name }),
            type: 'success',
          })
          onDone()
        },
      },
    )
  }

  const error = create.isError
    ? create.error.message
    : probe.isError
      ? probe.error.message
      : null

  return (
    <form onSubmit={save} className="space-y-4">
      <p className="text-sm text-muted-foreground">{t(AUTH_KEYS[key])}</p>
      <FieldGroup className="gap-4">
        <Field>
          <FieldLabel htmlFor={id('name')}>
            {t('registryVendors.name')}
          </FieldLabel>
          <Input
            id={id('name')}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <FieldDescription>{t('registryVendors.nameHelp')}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={id('host')}>
            {t('registryVendors.host')}
          </FieldLabel>
          <Input
            id={id('host')}
            className="font-mono"
            value={host}
            onChange={(e) => setHost(e.target.value)}
            placeholder={vendor.host_hint ?? vendor.default_host}
          />
          {vendor.host_hint ? (
            <FieldDescription>{vendor.host_hint}</FieldDescription>
          ) : null}
        </Field>
        <Field>
          <FieldLabel htmlFor={id('username')}>
            {t('registryVendors.username')}
          </FieldLabel>
          <Input
            id={id('username')}
            autoComplete="off"
            autoCapitalize="off"
            spellCheck={false}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={id('password')}>
            {t('registryVendors.password')}
          </FieldLabel>
          <SecretInput
            id={id('password')}
            autoComplete="off"
            className="font-mono"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <FieldDescription>{t('registryVendors.neverShown')}</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor={id('expires')}>
            {t('registryVendors.expires')}
          </FieldLabel>
          <Input
            id={id('expires')}
            type="date"
            value={expiresAt}
            onChange={(e) => setExpiresAt(e.target.value)}
          />
          {vendor.token_lifetime_hours ? (
            <FieldDescription>
              {t('registryVendors.shortLived', {
                hours: vendor.token_lifetime_hours,
              })}
            </FieldDescription>
          ) : null}
        </Field>
      </FieldGroup>

      <section
        aria-label={t('registryVendors.scopesTitle')}
        className="space-y-1 rounded-lg border border-border bg-muted/30 p-3 text-xs"
      >
        <h3 className="flex items-center gap-1.5 text-sm font-medium">
          <ShieldCheckGlassIcon
            aria-hidden="true"
            className="size-5 text-tone-success"
          />
          {t('registryVendors.scopesTitle')}
        </h3>
        <p className="text-muted-foreground">{t(SCOPE_KEYS[key])}</p>
      </section>

      {result ? (
        <Alert variant={result.ok ? 'default' : 'destructive'}>
          {result.ok ? (
            <CircleCheckGlassIcon className="text-tone-success" size="1.4em" />
          ) : (
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
          )}
          <AlertDescription>{result.message}</AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.4em" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={!ready || create.isPending}>
          {create.isPending
            ? t('registryVendors.saving')
            : t('registryVendors.save')}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={!ready || probe.isPending}
          onClick={runTest}
        >
          <PlugGlassIcon size="1.4em" aria-hidden="true" />
          {probe.isPending
            ? t('registryVendors.testing')
            : t('registryVendors.test')}
        </Button>
      </div>
    </form>
  )
}
