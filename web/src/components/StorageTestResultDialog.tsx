import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  WrenchGlassIcon,
} from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import {
  useTestStorageDestination,
  useUpdateStorageDestination,
} from '../queries/storage'
import type {
  StorageDestination,
  StorageFix,
  StorageProbeResult,
  StoragePreset,
  UpdateStorageDestinationRequest,
} from '../types/storage'

type EditField =
  'accessKeyId' | 'secretAccessKey' | 'bucket' | 'region' | 'endpoint'

const FIX_FIELDS: Partial<Record<StorageFix, EditField[]>> = {
  edit_credentials: ['accessKeyId', 'secretAccessKey'],
  edit_policy: ['accessKeyId', 'secretAccessKey'],
  edit_bucket: ['bucket'],
  edit_region: ['region'],
  edit_endpoint: ['endpoint'],
}

const FIELD_PLACEHOLDER: Record<EditField, string> = {
  accessKeyId: '',
  secretAccessKey: '',
  bucket: '',
  region: 'us-east-1',
  endpoint: 'https://s3.example.com',
}

function baseRequest(
  dest: StorageDestination,
): UpdateStorageDestinationRequest {
  return {
    name: dest.name,
    preset: dest.preset as StoragePreset,
    bucket: dest.bucket,
    ...(dest.endpoint ? { endpoint: dest.endpoint } : {}),
    ...(dest.region ? { region: dest.region } : {}),
    ...(dest.account_id ? { account_id: dest.account_id } : {}),
    path_style: dest.path_style,
  }
}

export function StorageTestResultDialog({
  dest,
  initial,
  open,
  onOpenChange,
}: {
  dest: StorageDestination
  initial: StorageProbeResult
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('settings')
  const [result, setResult] = useState(initial)
  const [values, setValues] = useState<Record<EditField, string>>({
    accessKeyId: '',
    secretAccessKey: '',
    bucket: dest.bucket,
    region: dest.region ?? '',
    endpoint: dest.endpoint ?? '',
  })
  const test = useTestStorageDestination()
  const update = useUpdateStorageDestination(dest.id)
  const busy = test.isPending || update.isPending
  const fix = result.fix
  const fields = fix ? (FIX_FIELDS[fix] ?? []) : []
  const needsKey = fields.includes('accessKeyId')
  const canSubmit =
    !busy &&
    fields.every((f) =>
      f === 'accessKeyId' || f === 'secretAccessKey'
        ? values[f].trim() !== ''
        : f === 'region'
          ? true
          : values[f].trim() !== '',
    )

  function recheck() {
    test.mutate(dest.id, {
      onSuccess: (next) => {
        setResult(next)
        if (next.ok) {
          toast.add({
            title: t('storageDestinations.test.passed', { name: dest.name }),
            type: 'success',
          })
          onOpenChange(false)
        }
      },
    })
  }

  function applyFix() {
    if (!fix) return
    if (dest.kind !== 's3' || fix === 'reconnect_node') {
      recheck()
      return
    }
    const req = baseRequest(dest)
    if (fix === 'enable_path_style') req.path_style = true
    if (fix === 'edit_bucket') req.bucket = values.bucket.trim()
    if (fix === 'edit_region') req.region = values.region.trim()
    if (fix === 'edit_endpoint') req.endpoint = values.endpoint.trim()
    if (needsKey) {
      req.access_key_id = values.accessKeyId.trim()
      req.secret_access_key = values.secretAccessKey
    }
    if (fix === 'sync_clock' || fix === 'enable_node_storage') {
      recheck()
      return
    }
    update.mutate(req, { onSuccess: recheck })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t('storageDestinations.test.failedTitle', { name: dest.name })}
          </DialogTitle>
          <DialogDescription>{result.message}</DialogDescription>
        </DialogHeader>
        <ul className="space-y-1 text-sm">
          {result.steps.map((s) => (
            <li key={s.name} className="flex items-center gap-2">
              {s.ok ? (
                <CircleCheckGlassIcon
                  className="size-5 text-emerald-600"
                  aria-hidden="true"
                />
              ) : (
                <CircleXGlassIcon
                  className="size-5 text-destructive"
                  aria-hidden="true"
                />
              )}
              {t('storageDestinations.test.step', {
                name: s.name,
                status: s.ok
                  ? t('storageDestinations.test.ok')
                  : t('storageDestinations.test.failed'),
              })}
            </li>
          ))}
        </ul>
        {fix ? (
          <Alert>
            <WrenchGlassIcon className="size-5" aria-hidden="true" />
            <AlertTitle>{t(`storageDestinations.fix.${fix}.title`)}</AlertTitle>
            <AlertDescription>
              {t(`storageDestinations.fix.${fix}.description`)}
            </AlertDescription>
          </Alert>
        ) : null}
        {fields.map((f) => (
          <Field key={f}>
            <FieldLabel htmlFor={`storage-fix-${f}`}>
              {t(`storageDestinations.fix.fields.${f}`)}
            </FieldLabel>
            <Input
              id={`storage-fix-${f}`}
              type={f === 'secretAccessKey' ? 'password' : 'text'}
              className="font-mono"
              autoComplete={f === 'secretAccessKey' ? 'new-password' : 'off'}
              spellCheck={false}
              placeholder={FIELD_PLACEHOLDER[f]}
              value={values[f]}
              onChange={(e) => {
                setValues((prev) => ({ ...prev, [f]: e.target.value }))
              }}
            />
          </Field>
        ))}
        {update.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {update.error.message}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('storageDestinations.test.close')}
          </Button>
          {fix === 'reconnect_node' ? (
            <Button variant="outline" render={<Link to="/nodes" />}>
              {t('storageDestinations.fix.reconnect_node.action')}
            </Button>
          ) : null}
          <Button type="button" disabled={!canSubmit} onClick={applyFix}>
            {busy
              ? t('storageDestinations.test.checking')
              : fix && fix !== 'reconnect_node'
                ? t(`storageDestinations.fix.${fix}.action`)
                : t('storageDestinations.test.recheck')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
