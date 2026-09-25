import { useState } from 'react'
import { CloudArrowUpIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../lib/apiError'
import {
  useCreateStorageDestination,
  useStorageProviders,
} from '../queries/storage'
import { StorageProviderPicker } from './StorageProviderPicker'
import type { StoragePreset } from '../types/storage'

interface FormState {
  name: string
  preset: StoragePreset
  bucket: string
  region: string
  endpoint: string
  accountId: string
  accessKeyId: string
  secretAccessKey: string
  virtualHosted: boolean
  verify: boolean
}

const EMPTY: FormState = {
  name: '',
  preset: 'aws',
  bucket: '',
  region: '',
  endpoint: '',
  accountId: '',
  accessKeyId: '',
  secretAccessKey: '',
  virtualHosted: false,
  verify: true,
}

const ENDPOINT_HINT: Partial<Record<StoragePreset, string>> = {
  minio: 'https://minio.example.com:9000',
  custom: 'https://s3.example.com',
}

const REGION_HINT: Partial<Record<StoragePreset, string>> = {
  aws: 'us-east-1',
  b2: 'us-west-004',
  wasabi: 'us-east-1',
}

export function CreateStorageDestinationDialog() {
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<FormState>(EMPTY)
  const { data: providers } = useStorageProviders()
  const create = useCreateStorageDestination()
  const provider = providers.find((p) => p.id === form.preset)

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setForm(EMPTY)
      create.reset()
    }
  }

  const missing =
    form.name.trim() === '' ||
    form.bucket.trim() === '' ||
    form.accessKeyId.trim() === '' ||
    form.secretAccessKey === '' ||
    (provider?.needs_account_id === true && form.accountId.trim() === '') ||
    (provider?.needs_region === true &&
      form.region.trim() === '' &&
      !provider.default_region) ||
    (provider?.needs_endpoint === true && form.endpoint.trim() === '')

  function submit() {
    create.mutate(
      {
        name: form.name.trim(),
        preset: form.preset,
        bucket: form.bucket.trim(),
        ...(form.region.trim() ? { region: form.region.trim() } : {}),
        ...(form.endpoint.trim() ? { endpoint: form.endpoint.trim() } : {}),
        ...(form.accountId.trim() ? { account_id: form.accountId.trim() } : {}),
        ...(form.virtualHosted ? { path_style: false } : {}),
        access_key_id: form.accessKeyId.trim(),
        secret_access_key: form.secretAccessKey,
        verify: form.verify,
      },
      {
        onSuccess: (created) => {
          handleOpenChange(false)
          toast.add({
            title: `Storage destination "${created.name}" connected.`,
            type: 'success',
          })
        },
      },
    )
  }

  const notConfigured =
    create.error instanceof ApiError && create.error.status === 501

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>Add destination</DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <CloudArrowUpIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Add storage destination
          </DialogTitle>
          <DialogDescription>
            An S3-compatible bucket for log archives and backups. Credentials
            are encrypted and never shown again.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            if (!missing) submit()
          }}
        >
          <StorageProviderPicker
            providers={providers}
            value={form.preset}
            onChange={(p) => {
              set('preset', p)
            }}
          />
          <Field>
            <FieldLabel htmlFor="storage-name">Name</FieldLabel>
            <Input
              id="storage-name"
              placeholder="e.g. log-archive"
              value={form.name}
              onChange={(e) => {
                set('name', e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="storage-bucket">Bucket</FieldLabel>
            <Input
              id="storage-bucket"
              value={form.bucket}
              onChange={(e) => {
                set('bucket', e.target.value)
              }}
            />
          </Field>
          {provider?.needs_account_id ? (
            <Field>
              <FieldLabel htmlFor="storage-account">
                Cloudflare account ID
              </FieldLabel>
              <Input
                id="storage-account"
                className="font-mono"
                value={form.accountId}
                onChange={(e) => {
                  set('accountId', e.target.value)
                }}
              />
              <FieldDescription>
                The endpoint becomes https://&lt;account-id&gt;
                .r2.cloudflarestorage.com.
              </FieldDescription>
            </Field>
          ) : null}
          {provider?.needs_region || provider?.default_region ? (
            <Field>
              <FieldLabel htmlFor="storage-region">
                Region
                {provider.needs_region ? null : (
                  <span className="text-muted-foreground"> (optional)</span>
                )}
              </FieldLabel>
              <Input
                id="storage-region"
                placeholder={
                  provider.default_region || REGION_HINT[form.preset]
                }
                value={form.region}
                onChange={(e) => {
                  set('region', e.target.value)
                }}
              />
            </Field>
          ) : null}
          {provider?.needs_endpoint ? (
            <Field>
              <FieldLabel htmlFor="storage-endpoint">Endpoint</FieldLabel>
              <Input
                id="storage-endpoint"
                placeholder={ENDPOINT_HINT[form.preset]}
                value={form.endpoint}
                onChange={(e) => {
                  set('endpoint', e.target.value)
                }}
              />
              <FieldDescription>
                Endpoints on private networks are blocked unless the control
                plane sets APP_NOTIFY_ALLOW_PRIVATE_NETWORKS=true.
              </FieldDescription>
            </Field>
          ) : null}
          <Field>
            <FieldLabel htmlFor="storage-access-key">Access key ID</FieldLabel>
            <Input
              id="storage-access-key"
              className="font-mono"
              autoComplete="off"
              spellCheck={false}
              value={form.accessKeyId}
              onChange={(e) => {
                set('accessKeyId', e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="storage-secret-key">
              Secret access key
            </FieldLabel>
            <Input
              id="storage-secret-key"
              type="password"
              className="font-mono"
              autoComplete="new-password"
              value={form.secretAccessKey}
              onChange={(e) => {
                set('secretAccessKey', e.target.value)
              }}
            />
          </Field>
          <div className="flex items-center gap-2 text-sm">
            <Checkbox
              id="storage-verify"
              checked={form.verify}
              onCheckedChange={(v) => {
                set('verify', v === true)
              }}
            />
            <label htmlFor="storage-verify">
              Test the connection before saving (writes and deletes a probe
              object)
            </label>
          </div>
          <div className="flex items-center gap-2 text-sm">
            <Checkbox
              id="storage-virtual-hosted"
              checked={form.virtualHosted}
              onCheckedChange={(v) => {
                set('virtualHosted', v === true)
              }}
            />
            <label htmlFor="storage-virtual-hosted">
              Use virtual-hosted addressing instead of path style
            </label>
          </div>
          {notConfigured ? (
            <Alert variant="destructive">
              <AlertTitle>Storage is not configured on this server</AlertTitle>
              <AlertDescription>
                Set APP_MASTER_KEY and restart the control plane so credentials
                can be encrypted.
              </AlertDescription>
            </Alert>
          ) : create.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {create.error.message}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                handleOpenChange(false)
              }}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={missing || create.isPending}>
              {create.isPending ? 'Connecting...' : 'Connect'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
