import { useState } from 'react'
import { CloudIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { StatusPill } from './kit/StatusPill'
import {
  useNodeProviders,
  useSetNodeProviderCredential,
} from '../queries/nodeProvision'
import type { NodeProviderResource } from '../types/nodeProvision'

const PROVIDER_LABELS: Record<string, string> = {
  hetzner: 'Hetzner',
  digitalocean: 'DigitalOcean',
}

// Instance-level cloud provider credentials for "nodes provision" and the
// nodes page's own Add node wizard: GET/POST /api/v1/node-providers. One
// mini-form per known provider, the same "list of independent cards"
// shape RegistryCredentialTable establishes for a different multi-
// credential resource, simplified here since there are only ever two
// rows and neither can be deleted (only replaced).
export function NodeProviderCredentialsCard() {
  const providers = useNodeProviders()
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CloudIcon className="size-4" />
          Cloud node provisioning
        </CardTitle>
        <CardDescription>
          API tokens for creating servers automatically from the Nodes page.
          Used only to create and inspect VMs; day to day operation never
          touches SSH.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {providers.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : providers.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{providers.error.message}</AlertDescription>
          </Alert>
        ) : (
          (providers.data ?? []).map((p) => (
            <ProviderForm key={p.provider} provider={p} />
          ))
        )}
      </CardContent>
    </Card>
  )
}

function ProviderForm({ provider }: { provider: NodeProviderResource }) {
  const [token, setToken] = useState('')
  const setCredential = useSetNodeProviderCredential()
  const label = PROVIDER_LABELS[provider.provider] ?? provider.provider

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!token) return
    setCredential.mutate(
      { provider: provider.provider, token },
      { onSuccess: () => setToken('') },
    )
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="space-y-3 rounded-lg border border-border p-3"
    >
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium text-foreground">{label}</span>
        <StatusPill
          tone={provider.has_token ? 'success' : 'neutral'}
          label={provider.has_token ? 'connected' : 'not connected'}
          size="sm"
        />
      </div>
      <Field>
        <FieldLabel htmlFor={`node-provider-token-${provider.provider}`}>
          API token
        </FieldLabel>
        <Input
          id={`node-provider-token-${provider.provider}`}
          type="password"
          autoComplete="off"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder={
            provider.has_token
              ? '••••••••••••'
              : `Paste your ${label} API token`
          }
        />
        <FieldDescription>
          {provider.has_token
            ? 'A token is already stored. Paste a new one to replace it.'
            : 'Never echoed back once saved.'}
        </FieldDescription>
      </Field>
      {setCredential.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{setCredential.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button
        type="submit"
        size="sm"
        disabled={!token || setCredential.isPending}
      >
        {setCredential.isPending ? 'Saving...' : 'Save'}
      </Button>
    </form>
  )
}
