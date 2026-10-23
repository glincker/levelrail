import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { HardDrivesIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SecretInput } from '@/components/ui/secret-input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { BrandTile } from '../BrandLogoBadge'
import { backupCoverageKeys } from '../../queries/backupCoverage'
import { useCreateBackupTarget } from '../../queries/backupTargets'
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
  const { t } = useTranslation('backups')
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
                <HardDrivesIcon aria-hidden="true" className="size-5" />
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
          <WarningIcon aria-hidden="true" />
          <AlertDescription>{t('target.localWarning')}</AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2">
        <FormField
          id="target-name"
          label={t('target.field.name')}
          invalid={missing === 'name'}
        >
          <Input
            id="target-name"
            value={values.name}
            placeholder={t('target.placeholder.name')}
            onChange={(e) => {
              set('name', e.target.value)
            }}
          />
        </FormField>
        <FormField
          id="target-bucket"
          label={t('target.field.bucket')}
          invalid={missing === 'bucket'}
        >
          <Input
            id="target-bucket"
            value={values.bucket}
            autoComplete="off"
            onChange={(e) => {
              set('bucket', e.target.value)
            }}
          />
        </FormField>
        <FormField id="target-region" label={t('target.field.region')}>
          <Input
            id="target-region"
            value={values.region}
            placeholder={preset.region}
            onChange={(e) => {
              set('region', e.target.value)
            }}
          />
        </FormField>
        {preset.provider !== 'aws' ? (
          <FormField
            id="target-endpoint"
            label={t('target.field.endpoint')}
            invalid={missing === 'endpoint'}
          >
            <Input
              id="target-endpoint"
              value={values.endpoint}
              placeholder={endpointPlaceholder}
              onChange={(e) => {
                set('endpoint', e.target.value)
              }}
            />
          </FormField>
        ) : null}
        <FormField
          id="target-key"
          label={t('target.field.accessKeyId')}
          invalid={missing === 'accessKeyId'}
        >
          <Input
            id="target-key"
            value={values.accessKeyId}
            autoComplete="off"
            onChange={(e) => {
              set('accessKeyId', e.target.value)
            }}
          />
        </FormField>
        <FormField
          id="target-secret"
          label={t('target.field.secretAccessKey')}
          invalid={missing === 'secretAccessKey'}
        >
          <SecretInput
            id="target-secret"
            value={values.secretAccessKey}
            autoComplete="new-password"
            onChange={(e) => {
              set('secretAccessKey', e.target.value)
            }}
          />
        </FormField>
      </div>

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

function FormField({
  id,
  label,
  invalid,
  children,
}: {
  id: string
  label: string
  invalid?: boolean
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1.5" data-invalid={invalid ? 'true' : undefined}>
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}
