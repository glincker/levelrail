import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ServerGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { SecretInput } from '@/components/ui/secret-input'
import { Field, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { BrandTile } from '../BrandLogoBadge'
import { backupTargetFieldsQueryOptions } from '../../queries/fieldDescriptors'
import { DescriptorFields } from '../DescriptorFields'
import { backupCoverageKeys } from '../../queries/backupCoverage'
import {
  backupTargetListQueryOptions,
  useCreateBackupTarget,
} from '../../queries/backupTargets'
import type { BackupTarget } from '../../types/backupTarget'
import {
  buildTargetRequest,
  presetBrand,
  presetById,
  resolveEndpoint,
  TARGET_PRESETS,
  type PresetId,
  type TargetFormValues,
} from './targetPresets'

// The S3 style fields every preset shares, in form order.
const S3_FIELD_IDS = [
  'name',
  's3.bucket',
  's3.region',
  's3.endpoint',
  's3.access_key_id',
  's3.secret_access_key',
]

// Wire key of a descriptor to the form value it edits.
const FORM_KEY: Record<string, keyof TargetFormValues> = {
  name: 'name',
  bucket: 'bucket',
  region: 'region',
  endpoint: 'endpoint',
  access_key_id: 'accessKeyId',
  secret_access_key: 'secretAccessKey',
}
function descriptorValues(v: TargetFormValues): Record<string, string> {
  return {
    name: v.name,
    bucket: v.bucket,
    region: v.region,
    endpoint: v.endpoint,
    access_key_id: v.accessKeyId,
    secret_access_key: v.secretAccessKey,
  }
}

const EMPTY: TargetFormValues = {
  name: '',
  bucket: '',
  region: '',
  endpoint: '',
  accessKeyId: '',
  secretAccessKey: '',
}

/** TargetSetupForm connects an S3-compatible destination from a provider preset. */
export function TargetSetupForm({
  onCreated,
}: {
  onCreated: (target: BackupTarget) => void
}) {
  const { data: existing } = useQuery(backupTargetListQueryOptions())
  const localRoot = existing?.find((x) => x.local_root)?.local_root
  const { t } = useTranslation('backups')
  const { t: tKey } = useTranslation()
  const { data: registry } = useQuery({
    ...backupTargetFieldsQueryOptions(),
    retry: false,
  })
  const descriptors = (registry?.fields ?? [])
    .filter((f) => S3_FIELD_IDS.includes(f.id))
    // Visibility is decided here by preset, the registry rule keys on the dialog's provider.
    .map((f) => ({ ...f, show_when: undefined }))
  const create = useCreateBackupTarget()
  const queryClient = useQueryClient()
  const [presetId, setPresetId] = useState<PresetId>('r2')
  const [values, setValues] = useState<TargetFormValues>(EMPTY)
  const [missing, setMissing] = useState<keyof TargetFormValues | null>(null)
  const preset = presetById(presetId)

  function set<K extends keyof TargetFormValues>(key: K, v: string) {
    setValues((prev) => ({ ...prev, [key]: v }))
    if (missing === key) setMissing(null)
  }

  function submit(event: React.FormEvent) {
    event.preventDefault()
    const built = buildTargetRequest(preset, values)
    if ('missing' in built) {
      setMissing(built.missing)
      return
    }
    create.mutate(built.request, {
      onSuccess: (target) => {
        void queryClient.invalidateQueries({
          queryKey: backupCoverageKeys.all,
        })
        toast.add({
          title: t('target.connected', { name: target.name }),
          type: 'success',
        })
        onCreated(target)
      },
    })
  }

  const endpointPlaceholder =
    resolveEndpoint(preset, '', values.region) || preset.endpoint

  return (
    <form
      className="space-y-4"
      onSubmit={submit}
      aria-label={t('target.formLabel')}
    >
      <div
        role="radiogroup"
        aria-label={t('target.providerLabel')}
        className="grid grid-cols-2 gap-2 sm:grid-cols-3"
      >
        {TARGET_PRESETS.map((p) => (
          <button
            key={p.id}
            type="button"
            role="radio"
            aria-checked={p.id === presetId}
            onClick={() => {
              setPresetId(p.id)
              setValues((prev) => ({ ...prev, region: '', endpoint: '' }))
            }}
            className={cn(
              'flex items-center gap-3 rounded-lg border px-3 py-3 text-left text-sm transition-colors focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
              p.id === presetId
                ? 'border-primary bg-primary/5 ring-1 ring-primary/30'
                : 'border-border hover:bg-muted',
            )}
          >
            <BrandTile
              name={presetBrand(p.id)}
              label={t(`target.preset.${p.id}.name`)}
            >
              {p.id === 'local' ? (
                <ServerGlassIcon aria-hidden="true" className="size-6" />
              ) : undefined}
            </BrandTile>
            <span className="min-w-0">
              <span className="block truncate font-medium">
                {t(`target.preset.${p.id}.name`)}
              </span>
              <span className="block truncate text-xs text-muted-foreground">
                {t(`target.preset.${p.id}.hint`)}
              </span>
            </span>
          </button>
        ))}
      </div>

      {preset.sameMachine ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon
            className="text-tone-warning"
            size="1.7em"
            aria-hidden="true"
          />
          <AlertDescription>{t('target.localWarning')}</AlertDescription>
        </Alert>
      ) : null}

      {descriptors.length === 0 ? (
        <Alert variant="destructive">
          <AlertDescription>{t('target.fieldsUnavailable')}</AlertDescription>
        </Alert>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          <DescriptorFields
            fields={descriptors}
            values={descriptorValues(values)}
            idPrefix="target"
            onChange={(key, v) => {
              const formKey = FORM_KEY[key]
              if (formKey) set(formKey, v)
            }}
            placeholder={(f) => {
              if (f.id === 'name') return t('target.placeholder.name')
              if (f.id === 's3.region') return preset.region
              if (f.id === 's3.endpoint') return endpointPlaceholder
              return undefined
            }}
            renderField={(f) => {
              if (f.id === 's3.endpoint' && preset.provider === 'aws')
                return null
              if (f.id !== 's3.secret_access_key') return undefined
              return (
                <Field key={f.id}>
                  <FieldLabel htmlFor="target-secret">
                    {(tKey as (key: string) => string)(f.label_key)}
                  </FieldLabel>
                  <SecretInput
                    id="target-secret"
                    value={values.secretAccessKey}
                    autoComplete="new-password"
                    onChange={(e) => {
                      set('secretAccessKey', e.target.value)
                    }}
                  />
                </Field>
              )
            }}
            fieldExtra={(f) =>
              f.id === 's3.bucket' && preset.sameMachine && localRoot ? (
                <p className="text-xs text-muted-foreground">
                  {t('target.localRootHint', { root: localRoot })}
                </p>
              ) : null
            }
          />
        </div>
      )}

      {missing ? (
        <p role="alert" className="text-sm text-destructive">
          {t('target.missing', { field: t(`target.field.${missing}`) })}
        </p>
      ) : null}
      {create.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {create.error.message}
        </p>
      ) : null}

      <Button type="submit" disabled={create.isPending}>
        {create.isPending ? t('target.connecting') : t('target.connect')}
      </Button>
    </form>
  )
}
