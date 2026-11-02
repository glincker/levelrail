import { useMemo, type Dispatch, type SetStateAction } from 'react'
import { useTranslation } from 'react-i18next'
import { LoaderGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useSpecValidation } from '../queries/specValidation'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
import {
  TRIGGER_MODES,
  duplicateServiceNames,
  servicesSpecYaml,
  type AdditionalServiceRow,
  type FormState,
  type ServiceSpecRow,
} from '../lib/gitSourceForm'
import { GitRepoSourcePicker } from './GitRepoSourcePicker'
import { GitSourceBuildPackField } from './GitSourceBuildPackField'
import {
  AdditionalServicesField,
  ServicesMapField,
} from './GitSourceServicesFields'

function matchesService(path: string, name: string): boolean {
  return path === `services.${name}` || path.startsWith(`services.${name}.`)
}

function patchRow<T>(rows: T[], index: number, patch: Partial<T>): T[] {
  return rows.map((row, i) => (i === index ? { ...row, ...patch } : row))
}

export function GitSourceEditForm({
  appName,
  form,
  setForm,
  error,
  setError,
  pending,
  notConnected,
  onSubmit,
  onCancel,
}: {
  appName: string
  form: FormState
  setForm: Dispatch<SetStateAction<FormState>>
  error: string | null
  setError: (message: string | null) => void
  pending: boolean
  notConnected: boolean
  onSubmit: () => void
  onCancel: () => void
}) {
  const { t } = useTranslation('gitSource')
  // Live validation for the services rows: the same checks a deploy-spec
  // call would hit, debounced so it fires once typing settles.
  const servicesYaml = useMemo(
    () =>
      form.fanoutMode === 'services' ? servicesSpecYaml(form.services) : '',
    [form.fanoutMode, form.services],
  )
  const debouncedServicesYaml = useDebouncedValue(servicesYaml, 500)
  const specValidation = useSpecValidation(appName, debouncedServicesYaml)
  const specIssues = specValidation.data?.issues ?? []
  const duplicateNames = useMemo(
    () => duplicateServiceNames(form.services),
    [form.services],
  )
  const namedServiceRows = useMemo(
    () =>
      new Set(form.services.map((r) => r.serviceName.trim()).filter(Boolean)),
    [form.services],
  )
  // Issues above the per-service level have no row to render under, so
  // they are shown once, separately.
  const unmatchedSpecIssues = specIssues.filter(
    (issue) =>
      ![...namedServiceRows].some((name) => matchesService(issue.path, name)),
  )

  function updateAdditional(
    index: number,
    patch: Partial<AdditionalServiceRow>,
  ) {
    setForm((f) => ({
      ...f,
      additionalServices: patchRow(f.additionalServices, index, patch),
    }))
    setError(null)
  }

  function updateService(index: number, patch: Partial<ServiceSpecRow>) {
    setForm((f) => ({ ...f, services: patchRow(f.services, index, patch) }))
    setError(null)
  }

  return (
    <div className="space-y-4">
      {!notConnected ? (
        <p className="text-sm text-muted-foreground">
          {t('edit.connectedTo', { repo: form.repoUrl, branch: form.branch })}
        </p>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('edit.connectIntro')}
        </p>
      )}

      <GitRepoSourcePicker
        disabled={pending}
        onSelect={(value) => {
          setForm((current) => ({
            ...current,
            repoUrl: value.repoUrl,
            branch: value.branch,
            token: value.token ?? '',
            providerRef: value.providerRef,
          }))
        }}
      />

      <GitSourceBuildPackField
        buildType={form.buildType}
        buildPath={form.buildPath}
        disabled={pending}
        onBuildType={(buildType) => {
          setForm((f) => ({ ...f, buildType }))
        }}
        onBuildPath={(buildPath) => {
          setForm((f) => ({ ...f, buildPath }))
        }}
      />

      <Field>
        <FieldLabel htmlFor="git-source-trigger-mode">
          {t('edit.deployTrigger')}
        </FieldLabel>
        <Select
          value={form.triggerMode}
          onValueChange={(v) => {
            if (v === 'push' || v === 'release') {
              setForm((f) => ({ ...f, triggerMode: v }))
            }
          }}
        >
          <SelectTrigger id="git-source-trigger-mode" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TRIGGER_MODES.map((mode) => (
              <SelectItem key={mode} value={mode}>
                {t(`triggers.${mode}.label`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <FieldDescription>
          {t(`triggers.${form.triggerMode}.description`)}
        </FieldDescription>
      </Field>

      <Field>
        <FieldLabel htmlFor="git-source-fanout-mode">
          {t('edit.fanoutMode')}
        </FieldLabel>
        <Tabs
          value={form.fanoutMode}
          onValueChange={(v: unknown) => {
            if (v === 'additional' || v === 'services') {
              setForm((f) => ({ ...f, fanoutMode: v }))
              setError(null)
            }
          }}
        >
          <TabsList
            id="git-source-fanout-mode"
            className="grid w-full grid-cols-2"
          >
            <TabsTrigger value="additional" disabled={pending}>
              {t('edit.fanoutAdditional')}
            </TabsTrigger>
            <TabsTrigger value="services" disabled={pending}>
              {t('edit.fanoutServices')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </Field>

      {form.fanoutMode === 'additional' ? (
        <AdditionalServicesField
          rows={form.additionalServices}
          disabled={pending}
          error={error}
          onAdd={() => {
            setForm((f) => ({
              ...f,
              additionalServices: [
                ...f.additionalServices,
                { serviceName: '', buildType: 'dockerfile', buildPath: '' },
              ],
            }))
          }}
          onUpdate={updateAdditional}
          onRemove={(index) => {
            setForm((f) => ({
              ...f,
              additionalServices: f.additionalServices.filter(
                (_, i) => i !== index,
              ),
            }))
            setError(null)
          }}
        />
      ) : (
        <ServicesMapField
          appName={appName}
          rows={form.services}
          disabled={pending}
          error={error}
          duplicateNames={duplicateNames}
          issuesForService={(name) =>
            specIssues.filter((issue) => matchesService(issue.path, name))
          }
          unmatchedIssues={unmatchedSpecIssues}
          checking={specValidation.isFetching}
          onAdd={() => {
            setForm((f) => ({
              ...f,
              services: [
                ...f.services,
                {
                  serviceName: '',
                  buildType: 'dockerfile',
                  buildPath: '',
                  port: '',
                },
              ],
            }))
          }}
          onUpdate={updateService}
          onRemove={(index) => {
            setForm((f) => ({
              ...f,
              services: f.services.filter((_, i) => i !== index),
            }))
            setError(null)
          }}
        />
      )}

      <div className="flex gap-2">
        <Button type="button" size="sm" disabled={pending} onClick={onSubmit}>
          {pending ? <LoaderGlassIcon className="size-5 animate-spin" /> : null}
          {notConnected ? t('edit.connect') : t('edit.save')}
        </Button>
        {!notConnected ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={pending}
            onClick={onCancel}
          >
            {t('edit.cancel')}
          </Button>
        ) : null}
      </div>
    </div>
  )
}
