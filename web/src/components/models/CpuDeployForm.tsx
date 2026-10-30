import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import {
  formatCpus,
  formatGiB,
  formatMemoryMiB,
  suggestedName,
} from '../../lib/modelsCpu'
import { useCreateModel } from '../../queries/models'
import { useCpuFit, useLocalModelCatalog } from '../../queries/modelsCpu'
import type { CreateModelResponse } from '../../types/models'
import type { CpuFitRequest } from '../../types/modelsCpu'
import { CpuFitSummary } from './CpuFitSummary'
import { CpuLimitSliders } from './CpuLimitSliders'
import { CpuModelCards } from './CpuModelCards'

const NAME_PATTERN = /^[a-z0-9]([a-z0-9-]{0,40}[a-z0-9])?$/
const POSITIVE_INT = /^[1-9][0-9]*$/
const CUSTOM = 'custom'
const FIT_DEBOUNCE_MS = 300
// Used only when the host has not reported its CPUs and memory.
const FALLBACK_BOUNDS = {
  default_cpu_milli: 2000,
  default_memory_mib: 4096,
  max_cpu_milli: 8000,
  max_memory_mib: 32768,
  free_ram_floor_mib: 1024,
}

export function CpuDeployForm({
  onDeployed,
  onCancel,
}: {
  onDeployed: (created: CreateModelResponse) => void
  onCancel: () => void
}) {
  const { t } = useTranslation('models')
  const catalog = useLocalModelCatalog('')
  const create = useCreateModel()
  const [choice, setChoice] = useState('')
  const [name, setName] = useState<string | null>(null)
  const [customRepo, setCustomRepo] = useState('')
  const [customFile, setCustomFile] = useState('')
  const [ack, setAck] = useState(false)
  const [cpuOverride, setCpuOverride] = useState<number | null>(null)
  const [memOverride, setMemOverride] = useState<number | null>(null)
  const [unload, setUnload] = useState(true)
  const [idleMinutes, setIdleMinutes] = useState('')
  const [submitted, setSubmitted] = useState(false)

  const cards = catalog.data?.models ?? []
  const defaultId = cards.find((c) => c.tier === 'default')?.id ?? cards[0]?.id
  const selected = choice || defaultId || ''
  const custom = selected === CUSTOM
  const bounds =
    catalog.data && catalog.data.bounds.max_cpu_milli > 0
      ? catalog.data.bounds
      : FALLBACK_BOUNDS
  const cpuMilli = cpuOverride ?? bounds.default_cpu_milli
  const memoryMiB = memOverride ?? bounds.default_memory_mib
  const effectiveName = name ?? (custom ? '' : suggestedName(selected))

  const request = useDebouncedValue<CpuFitRequest>(
    custom
      ? {
          compute: 'cpu',
          model: customRepo.trim(),
          cpus: cpuMilli / 1000,
          memory_mib: memoryMiB,
        }
      : {
          compute: 'cpu',
          catalog_id: selected,
          cpus: cpuMilli / 1000,
          memory_mib: memoryMiB,
        },
    FIT_DEBOUNCE_MS,
  )
  const fitReady = custom ? customRepo.trim() !== '' : selected !== ''
  const fit = useCpuFit(request, fitReady)

  const nameOk = NAME_PATTERN.test(effectiveName)
  const idleOk = !unload || idleMinutes === '' || POSITIVE_INT.test(idleMinutes)
  const licenseOk = !custom || ack
  const blocked = fit.data?.verdict === 'wont_fit'
  const errors = {
    name: submitted && !nameOk,
    repo: submitted && custom && customRepo.trim() === '',
    idle: submitted && !idleOk,
    license: submitted && !licenseOk,
  }
  const apiError = create.isError ? create.error.message : null

  function submit() {
    setSubmitted(true)
    if (!nameOk || !idleOk || !licenseOk || !fitReady || blocked) return
    const entry = cards.find((c) => c.id === selected)
    create.mutate(
      {
        name: effectiveName,
        engine: 'llamacpp',
        model: custom
          ? customRepo.trim()
          : `${entry?.repo ?? ''}:${entry?.quant ?? ''}`,
        compute: 'cpu',
        catalog_id: custom ? undefined : selected,
        weights_file:
          custom && customFile.trim() ? customFile.trim() : undefined,
        accept_license: custom ? ack : undefined,
        cpus: cpuMilli / 1000,
        memory_mib: memoryMiB,
        residency: unload ? 'on_demand' : 'always',
        idle_ttl_seconds:
          unload && idleMinutes !== '' ? Number(idleMinutes) * 60 : undefined,
      },
      {
        onSuccess: (created) => {
          toast.add({
            title: t('deploy.toast.title', { name: created.name }),
            description: t('cpu.deploy.toast'),
            type: 'success',
          })
          onDeployed(created)
        },
      },
    )
  }

  if (catalog.isPending) {
    return <Skeleton className="h-64 w-full" />
  }
  if (catalog.isError) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {catalog.error.message}
      </p>
    )
  }

  const disk = fit.data?.disk_bytes ?? 0
  return (
    <form
      noValidate
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <CpuModelCards
        legend={t('cpu.deploy.pick')}
        cards={cards}
        value={selected}
        onChange={(id) => {
          setChoice(id)
          setName(null)
          setAck(false)
        }}
      />
      <p className="text-xs text-muted-foreground">
        {t('cpu.deploy.catalogNote', { date: catalog.data.verified_on })}
      </p>
      <label className="flex items-center gap-2 text-sm">
        <input
          type="radio"
          name="cpu-model"
          value={CUSTOM}
          checked={custom}
          onChange={() => {
            setChoice(CUSTOM)
            setName(null)
          }}
        />
        {t('cpu.deploy.useCustom')}
      </label>
      {custom ? (
        <div className="space-y-3 rounded-lg border border-border p-3">
          <Field>
            <FieldLabel htmlFor="cpu-custom-repo">
              {t('cpu.deploy.customRepo')}
            </FieldLabel>
            <Input
              id="cpu-custom-repo"
              value={customRepo}
              onChange={(e) => {
                setCustomRepo(e.target.value)
              }}
              placeholder="org/name-GGUF:Q4_K_M"
              className="font-mono"
              autoComplete="off"
              aria-invalid={errors.repo}
            />
            <FieldHint>{t('cpu.deploy.customRepoHint')}</FieldHint>
          </Field>
          <Field>
            <FieldLabel htmlFor="cpu-custom-file">
              {t('cpu.deploy.customFile')}
            </FieldLabel>
            <Input
              id="cpu-custom-file"
              value={customFile}
              onChange={(e) => {
                setCustomFile(e.target.value)
              }}
              placeholder="model-Q4_K_M.gguf"
              className="font-mono"
              autoComplete="off"
            />
            <FieldHint>{t('cpu.deploy.customFileHint')}</FieldHint>
          </Field>
          <Alert>
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.7em"
              aria-hidden="true"
            />
            <AlertTitle>{t('cpu.license.title')}</AlertTitle>
            <AlertDescription>
              {fit.data?.license_notice ?? t('cpu.license.generic')}
            </AlertDescription>
          </Alert>
          <label className="flex items-start gap-2 text-sm">
            <Checkbox
              checked={ack}
              onCheckedChange={(v) => {
                setAck(v === true)
              }}
              aria-invalid={errors.license}
            />
            <span>{t('cpu.license.ack')}</span>
          </label>
          {errors.license ? (
            <p role="alert" className="text-xs text-destructive">
              {t('cpu.license.required')}
            </p>
          ) : null}
        </div>
      ) : null}

      <CpuLimitSliders
        bounds={bounds}
        cpuMilli={cpuMilli}
        memoryMiB={memoryMiB}
        onCpu={setCpuOverride}
        onMemory={setMemOverride}
      />
      {fit.data ? (
        <CpuFitSummary
          fit={fit.data}
          onPickSuggested={(id) => {
            setChoice(id)
            setName(null)
          }}
        />
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="cpu-model-name">
            {t('deploy.fields.name')}
          </FieldLabel>
          <Input
            id="cpu-model-name"
            value={effectiveName}
            onChange={(e) => {
              setName(e.target.value)
            }}
            autoComplete="off"
            aria-invalid={errors.name}
          />
          {errors.name ? (
            <p role="alert" className="text-xs text-destructive">
              {t('deploy.errors.name')}
            </p>
          ) : (
            <FieldHint>{t('deploy.fields.nameHint')}</FieldHint>
          )}
        </Field>
        <Field>
          <label className="flex items-center justify-between gap-3 text-sm font-medium">
            {t('cpu.deploy.unload')}
            <Switch checked={unload} onCheckedChange={setUnload} />
          </label>
          <FieldHint>{t('cpu.deploy.unloadHint')}</FieldHint>
          {unload ? (
            <Input
              aria-label={t('deploy.fields.idleMinutes')}
              value={idleMinutes}
              onChange={(e) => {
                setIdleMinutes(e.target.value)
              }}
              placeholder={t('deploy.fields.idlePlaceholder')}
              inputMode="numeric"
              aria-invalid={errors.idle}
            />
          ) : null}
        </Field>
      </div>

      <Alert>
        <TriangleAlertGlassIcon
          className="text-tone-warning"
          size="1.7em"
          aria-hidden="true"
        />
        <AlertTitle>{t('cpu.deploy.warnTitle')}</AlertTitle>
        <AlertDescription>
          {t('cpu.deploy.warn', {
            cpus: formatCpus(cpuMilli),
            memory: formatMemoryMiB(memoryMiB),
            disk: disk > 0 ? formatGiB(disk) : t('cpu.deploy.diskUnknown'),
          })}
        </AlertDescription>
      </Alert>
      {apiError ? (
        <p role="alert" className="text-sm text-destructive">
          {apiError}
        </p>
      ) : null}
      <div className="sticky bottom-0 -mx-4 -mb-4 flex flex-wrap items-center justify-end gap-2 border-t border-border bg-popover px-4 py-3">
        <Button type="button" variant="outline" onClick={onCancel}>
          {t('deploy.actions.cancel')}
        </Button>
        <Button type="submit" disabled={create.isPending || blocked}>
          {create.isPending
            ? t('deploy.actions.deploying')
            : t('cpu.deploy.submit')}
        </Button>
      </div>
    </form>
  )
}
