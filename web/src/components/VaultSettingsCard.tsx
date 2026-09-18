import { useState, type FormEvent } from 'react'
import { VaultIcon } from '@phosphor-icons/react/dist/ssr'
import type { VaultSettings } from '../queries/vault'
import {
  useDisconnectVault,
  useUpdateVaultSettings,
} from '../queries/vault'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  SettingsEnabledRow,
  SettingsFormActions,
  SettingsFormAlerts,
} from './SettingsCard'

// Instance-level external HashiCorp Vault connection: GET/PUT/DELETE
// /api/v1/settings/vault. Not built on useTokenToggleForm
// (SettingsCard.tsx): that hook's TokenSettingsRequest shape is
// {enabled, token?} only, too narrow for Vault's address/auth_method/
// namespace/role_id/credential fields, so this owns its own form state
// instead, following the same enabled/credential/formError/pending
// shape that hook establishes for the two simpler cards.
export function VaultSettingsCard({ settings }: { settings: VaultSettings }) {
  const updateSettings = useUpdateVaultSettings()
  const disconnect = useDisconnectVault()

  const [enabled, setEnabled] = useState(settings.enabled)
  const [address, setAddress] = useState(settings.address)
  const [authMethod, setAuthMethod] = useState<'token' | 'approle'>(
    settings.auth_method,
  )
  const [namespace, setNamespace] = useState(settings.namespace ?? '')
  const [roleID, setRoleID] = useState(settings.role_id ?? '')
  const [mountPath, setMountPath] = useState(settings.mount_path)
  const [credential, setCredential] = useState('')
  const [formError, setFormError] = useState<string | null>(null)

  const pending = updateSettings.isPending || disconnect.isPending

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setFormError(null)
    if (enabled && !address.trim()) {
      setFormError('A Vault address is required to enable Vault.')
      return
    }
    if (authMethod === 'approle' && !roleID.trim()) {
      setFormError('A role ID is required for AppRole auth.')
      return
    }
    if (enabled && !settings.has_credential && !credential.trim()) {
      setFormError(
        authMethod === 'approle'
          ? 'A secret ID is required the first time Vault is enabled with AppRole auth.'
          : 'A Vault token is required the first time Vault is enabled.',
      )
      return
    }
    updateSettings.mutate(
      {
        enabled,
        address: address.trim(),
        auth_method: authMethod,
        namespace: namespace.trim() || undefined,
        role_id: roleID.trim() || undefined,
        mount_path: mountPath.trim() || undefined,
        credential: credential.trim() || undefined,
      },
      {
        onSuccess: () => {
          setCredential('')
        },
      },
    )
  }

  function handleDisconnect() {
    disconnect.mutate(undefined, {
      onSuccess: () => {
        setEnabled(false)
        setCredential('')
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <VaultIcon className="size-4" />
          HashiCorp Vault
        </CardTitle>
        <CardDescription>
          Resolve an app.yaml env var&apos;s value live from an external
          HashiCorp Vault instance ({'{'} vault: {'{'} path, key {'}'} {'}'}),
          as an alternative to this platform&apos;s own envelope-encrypted
          secret storage.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={handleSubmit} className="space-y-5">
          <SettingsEnabledRow
            description="Resolve vault-backed app.yaml env vars at container-create time."
            checked={enabled}
            onCheckedChange={setEnabled}
            disabled={pending}
            ariaLabel="Vault enabled"
          />

          <Field>
            <FieldLabel htmlFor="vault-address">Vault address</FieldLabel>
            <Input
              id="vault-address"
              placeholder="https://vault.internal:8200"
              autoComplete="off"
              value={address}
              onChange={(e) => {
                setAddress(e.target.value)
              }}
              disabled={pending}
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="vault-auth-method">Auth method</FieldLabel>
            <Select
              value={authMethod}
              onValueChange={(v) => {
                setAuthMethod(v as 'token' | 'approle')
              }}
              disabled={pending}
            >
              <SelectTrigger id="vault-auth-method">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="token">Token</SelectItem>
                <SelectItem value="approle">AppRole</SelectItem>
              </SelectContent>
            </Select>
          </Field>

          {authMethod === 'approle' ? (
            <Field>
              <FieldLabel htmlFor="vault-role-id">Role ID</FieldLabel>
              <Input
                id="vault-role-id"
                autoComplete="off"
                value={roleID}
                onChange={(e) => {
                  setRoleID(e.target.value)
                }}
                disabled={pending}
              />
              <FieldDescription>
                Not itself sensitive; shown here as plain text.
              </FieldDescription>
            </Field>
          ) : null}

          <Field>
            <FieldLabel htmlFor="vault-credential">
              {authMethod === 'approle' ? 'Secret ID' : 'Vault token'}
            </FieldLabel>
            <Input
              id="vault-credential"
              type="password"
              autoComplete="off"
              value={credential}
              onChange={(e) => {
                setCredential(e.target.value)
              }}
              disabled={pending}
              placeholder={
                settings.has_credential ? '••••••••••••' : 'Paste a credential'
              }
            />
            <FieldDescription>
              {settings.has_credential
                ? 'A credential is already configured. Leave blank to keep it.'
                : 'No credential set yet.'}
            </FieldDescription>
          </Field>

          <Field>
            <FieldLabel htmlFor="vault-namespace">
              Namespace (optional)
            </FieldLabel>
            <Input
              id="vault-namespace"
              autoComplete="off"
              value={namespace}
              onChange={(e) => {
                setNamespace(e.target.value)
              }}
              disabled={pending}
              placeholder="Vault Enterprise namespace, if any"
            />
          </Field>

          <Field>
            <FieldLabel htmlFor="vault-mount-path">
              KV v2 mount path
            </FieldLabel>
            <Input
              id="vault-mount-path"
              autoComplete="off"
              value={mountPath}
              onChange={(e) => {
                setMountPath(e.target.value)
              }}
              disabled={pending}
              placeholder="secret"
            />
          </Field>

          <SettingsFormAlerts
            alerts={[
              formError ? { key: 'form', message: formError } : null,
              updateSettings.isError
                ? { key: 'update', message: updateSettings.error.message }
                : null,
              disconnect.isError
                ? { key: 'disconnect', message: disconnect.error.message }
                : null,
            ]}
          />

          <SettingsFormActions
            pending={pending}
            savePending={updateSettings.isPending}
            showSecondary={settings.enabled || settings.has_credential}
            secondaryPending={disconnect.isPending}
            secondaryLabel="Disconnect"
            secondaryPendingLabel="Disconnecting..."
            onSecondaryClick={handleDisconnect}
          />
        </form>
      </CardContent>
    </Card>
  )
}
