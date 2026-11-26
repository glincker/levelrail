import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  FolderGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { BrandIcon } from '../../BrandIcon'
import {
  useCreateBackupTarget,
  useDeleteBackupTarget,
  useTestBackupTarget,
} from '../../../queries/backupTargets'
import type { BackupTarget } from '../../../types/backupTarget'
import {
  buildTargetRequest,
  EMPTY_VALUES,
  PRESETS,
  type FieldId,
  type Preset,
  type PresetValues,
} from './backupTargetPresets'

// Where backups on the local disk go by default, under the control plane's backup root.
const LOCAL_DEFAULT = 'backups'

function Labeled({
  id,
  label,
  children,
}: Readonly<{ id: string; label: string; children: React.ReactNode }>) {
  return (
    <div className="grid items-center gap-1 sm:grid-cols-[9rem_1fr]">
      <label htmlFor={id} className="text-sm">
        {label}
      </label>
      {children}
    </div>
  )
}

/** BackupTargetSetup picks a storage provider, builds its target from a few fields and tests it once created. */
export function BackupTargetSetup({
  onCreated,
  onCancel,
}: Readonly<{
  onCreated: (target: BackupTarget) => void
  onCancel?: () => void
}>) {
  const { t } = useTranslation('dbBackupSetup')
  const create = useCreateBackupTarget()
  const test = useTestBackupTarget()
  const remove = useDeleteBackupTarget()
  const [preset, setPreset] = useState<Preset | null>(null)
  const [name, setName] = useState('')
  const [values, setValues] = useState<PresetValues>(EMPTY_VALUES)
  const [created, setCreated] = useState<BackupTarget | null>(null)

  function choose(p: Preset) {
    setPreset(p)
    setName(`${p.id}-backups`)
    setValues({
      ...EMPTY_VALUES,
      region: p.defaultRegion ?? '',
      path: p.id === 'local' ? LOCAL_DEFAULT : '',
    })
  }
  const set = (k: keyof PresetValues) => (e: { target: { value: string } }) => {
    setValues((v) => ({ ...v, [k]: e.target.value }))
  }

  function runTest(target: BackupTarget) {
    test.reset()
    test.mutate(target.id)
  }

  if (!preset) {
    return (
      <div className="space-y-2">
        <p className="text-sm font-medium">{t('gallery.title')}</p>
        <ul className="grid gap-2 sm:grid-cols-2">
          {PRESETS.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => {
                  choose(p)
                }}
                className="flex w-full items-center gap-2 rounded-md border px-2.5 py-2 text-left text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
              >
                {p.brand ? (
                  <BrandIcon name={p.brand} className="size-5 shrink-0" />
                ) : (
                  <FolderGlassIcon
                    className="size-5 shrink-0"
                    aria-hidden="true"
                  />
                )}
                {p.id === 'local' ? t('local') : p.label}
              </button>
            </li>
          ))}
        </ul>
        {onCancel ? (
          <Button type="button" variant="outline" size="sm" onClick={onCancel}>
            {t('cancel')}
          </Button>
        ) : null}
      </div>
    )
  }

  const request = buildTargetRequest(preset, name, values)
  const has = (f: FieldId) => preset.fields.includes(f)

  if (created) {
    return (
      <div className="space-y-3" role="status">
        {test.isPending ? (
          <p className="flex items-center gap-2 text-sm">
            <LoaderCircleGlassIcon
              className="size-5 animate-spin motion-reduce:animate-none"
              aria-hidden="true"
            />
            {t('testing')}
          </p>
        ) : test.isError ? (
          <p className="flex items-start gap-2 text-sm text-destructive">
            <CircleAlertGlassIcon
              className="mt-0.5 size-5"
              aria-hidden="true"
            />
            {t('testFailed', { error: test.error.message })}
          </p>
        ) : test.isSuccess ? (
          <p className="flex items-center gap-2 text-sm">
            <CircleCheckGlassIcon
              className="size-5 text-success"
              aria-hidden="true"
            />
            {t('testOk')}
          </p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          {test.isSuccess ? (
            <Button
              type="button"
              onClick={() => {
                onCreated(created)
              }}
            >
              {t('continue')}
            </Button>
          ) : null}
          {test.isError ? (
            <>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  runTest(created)
                }}
              >
                {t('testAgain')}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={remove.isPending}
                onClick={() => {
                  remove.mutate(created.id, {
                    onSuccess: () => {
                      setCreated(null)
                      test.reset()
                    },
                  })
                }}
              >
                {t('removeAndEdit')}
              </Button>
            </>
          ) : null}
        </div>
      </div>
    )
  }

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        if (!request) return
        create.mutate(request, {
          onSuccess: (target) => {
            setCreated(target)
            runTest(target)
          },
        })
      }}
    >
      <p className="flex items-center gap-2 text-sm font-medium">
        {preset.brand ? (
          <BrandIcon name={preset.brand} className="size-5" />
        ) : (
          <FolderGlassIcon className="size-5" aria-hidden="true" />
        )}
        {preset.id === 'local' ? t('local') : preset.label}
      </p>
      <Labeled id="bt-name" label={t('fields.name')}>
        <Input
          id="bt-name"
          value={name}
          onChange={(e) => {
            setName(e.target.value)
          }}
        />
      </Labeled>
      {has('accountId') ? (
        <Labeled id="bt-account" label={t('fields.accountId')}>
          <Input
            id="bt-account"
            value={values.accountId}
            autoComplete="off"
            onChange={set('accountId')}
          />
        </Labeled>
      ) : null}
      {has('region') ? (
        <Labeled id="bt-region" label={t('fields.region')}>
          <NativeSelect
            id="bt-region"
            value={values.region}
            onChange={set('region')}
          >
            {preset.regions?.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </NativeSelect>
        </Labeled>
      ) : null}
      {has('endpoint') ? (
        <Labeled id="bt-endpoint" label={t('fields.endpoint')}>
          <Input
            id="bt-endpoint"
            value={values.endpoint}
            placeholder="https://minio.example.com"
            onChange={set('endpoint')}
          />
        </Labeled>
      ) : null}
      {has('bucket') ? (
        <Labeled id="bt-bucket" label={t('fields.bucket')}>
          <Input
            id="bt-bucket"
            value={values.bucket}
            onChange={set('bucket')}
          />
        </Labeled>
      ) : null}
      {has('path') ? (
        <Labeled id="bt-path" label={t('fields.path')}>
          <Input id="bt-path" value={values.path} onChange={set('path')} />
        </Labeled>
      ) : null}
      {preset.id !== 'local' ? (
        <>
          <Labeled id="bt-key" label={t('fields.accessKey')}>
            <Input
              id="bt-key"
              value={values.accessKey}
              autoComplete="off"
              onChange={set('accessKey')}
            />
          </Labeled>
          <Labeled id="bt-secret" label={t('fields.secret')}>
            <Input
              id="bt-secret"
              type="password"
              value={values.secret}
              autoComplete="off"
              onChange={set('secret')}
            />
          </Labeled>
          <p className="text-xs text-muted-foreground">{t('secretNote')}</p>
        </>
      ) : null}
      {create.error ? (
        <p className="text-sm text-destructive" role="alert">
          {create.error.message}
        </p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          onClick={() => {
            setPreset(null)
            create.reset()
          }}
        >
          {t('back')}
        </Button>
        <Button type="submit" disabled={!request || create.isPending}>
          {create.isPending ? t('creating') : t('createAndTest')}
        </Button>
      </div>
    </form>
  )
}
