import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { MODEL_ENGINES, formatMiB, nodeGpuFacts } from '../lib/models'
import {
  INITIAL_DEPLOY_FORM,
  LOCAL_NODE,
  buildCreateRequest,
  deployFieldErrors,
  type DeployFormState,
} from '../lib/modelDeployForm'
import { useCreateModel, useGpuNodes } from '../queries/models'
import { ModelPreflightPanel } from './ModelPreflightPanel'
import { ModelFitPanel } from './models/ModelFitPanel'
import { DeployAdvancedFields } from './models/DeployAdvancedFields'
import { DeployEngineCards } from './models/DeployEngineCards'
import { CpuDeployForm } from './models/CpuDeployForm'
import { DeployComputeSwitch } from './models/DeployComputeSwitch'
import type { ModelCompute } from '../types/modelsCpu'
import type { CreateModelResponse, GpuNode } from '../types/models'

export function DeployModelDialog({
  onDeployed,
}: {
  onDeployed: (created: CreateModelResponse) => void
}) {
  const { t } = useTranslation('models')
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<DeployFormState>(INITIAL_DEPLOY_FORM)
  const [submitted, setSubmitted] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [compute, setCompute] = useState<ModelCompute>('gpu')
  const create = useCreateModel()
  const gpuNodes = useGpuNodes()
  const engine = MODEL_ENGINES.find((e) => e.id === form.engine)
  const errors = deployFieldErrors(form)
  const hasErrors = Object.keys(errors).length > 0
  const shown = submitted ? errors : {}

  function set<K extends keyof DeployFormState>(
    key: K,
    value: DeployFormState[K],
  ) {
    setForm((prev) => ({ ...prev, [key]: value }))
  }

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      setForm(INITIAL_DEPLOY_FORM)
      setSubmitted(false)
      setAdvancedOpen(false)
      setCompute('gpu')
      create.reset()
    }
  }

  function submit() {
    setSubmitted(true)
    if (hasErrors) {
      const advancedOnly =
        !errors.name &&
        !errors.model &&
        (errors.gpus || errors.context || errors.idle || errors.swapGroup)
      if (advancedOnly) setAdvancedOpen(true)
      return
    }
    create.mutate(buildCreateRequest(form), {
      onSuccess: (created) => {
        toast.add({
          title: t('deploy.toast.title', { name: created.name }),
          description: t('deploy.toast.description'),
          type: 'success',
        })
        handleOpenChange(false)
        onDeployed(created)
      },
    })
  }

  function nodeOptionText(node: GpuNode): string {
    if (node.devices.length === 0) {
      return `${node.name}, ${t('deploy.nodeOption.noGpu')}`
    }
    const facts = nodeGpuFacts(node)
    const free = t('deploy.nodeOption.freeVram', {
      free: formatMiB(facts.freeMib),
      total: formatMiB(facts.totalMib),
    })
    return `${node.name}, ${facts.gpuNames}, ${free}`
  }

  const usableNodes = (gpuNodes.data ?? []).filter((n) => n.present)
  const selectedNode = usableNodes.find((n) => n.node_id === form.node)
  const nodeText =
    form.node === LOCAL_NODE || !selectedNode
      ? t('deploy.fields.localNode')
      : selectedNode.name
  const summary =
    errors.name || errors.model
      ? t('deploy.summary.incomplete')
      : t('deploy.summary.ready', {
          name: form.name,
          engine: engine?.label ?? form.engine,
          model: form.model.trim(),
          node: nodeText,
        })
  const apiError = create.isError ? create.error.message : null

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button size="sm" />}>
        <PlusGlassIcon size="1.2em" aria-hidden="true" />
        {t('deploy.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('deploy.title')}</DialogTitle>
          <DialogDescription>
            {compute === 'cpu'
              ? t('cpu.deploy.description')
              : t('deploy.description')}
          </DialogDescription>
        </DialogHeader>
        <DeployComputeSwitch value={compute} onChange={setCompute} />
        {compute === 'cpu' ? (
          <CpuDeployForm
            onCancel={() => {
              handleOpenChange(false)
            }}
            onDeployed={(created) => {
              handleOpenChange(false)
              onDeployed(created)
            }}
          />
        ) : (
          <form
            noValidate
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault()
              submit()
            }}
          >
            <div className="grid gap-5 md:grid-cols-2">
              <div className="space-y-3">
                <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  {t('deploy.sections.basics')}
                </h3>
                <Field>
                  <FieldLabel htmlFor="model-name">
                    {t('deploy.fields.name')}
                  </FieldLabel>
                  <Input
                    id="model-name"
                    value={form.name}
                    onChange={(e) => {
                      set('name', e.target.value)
                    }}
                    placeholder={t('deploy.fields.namePlaceholder')}
                    autoComplete="off"
                    aria-invalid={shown.name === true}
                    aria-describedby="model-name-hint"
                  />
                  {shown.name ? (
                    <p
                      id="model-name-hint"
                      role="alert"
                      className="text-xs text-destructive"
                    >
                      {t('deploy.errors.name')}
                    </p>
                  ) : (
                    <FieldHint id="model-name-hint">
                      {t('deploy.fields.nameHint')}
                    </FieldHint>
                  )}
                </Field>
                <DeployEngineCards
                  legend={t('deploy.fields.engine')}
                  value={form.engine}
                  onChange={(next) => {
                    set('engine', next)
                  }}
                />
                <Field>
                  <FieldLabel htmlFor="model-ref">
                    {t('deploy.fields.model')}
                  </FieldLabel>
                  <Input
                    id="model-ref"
                    list="model-ref-suggestions"
                    value={form.model}
                    onChange={(e) => {
                      set('model', e.target.value)
                    }}
                    placeholder={engine?.refHint}
                    autoComplete="off"
                    className="font-mono"
                    aria-invalid={shown.model === true}
                    aria-describedby="model-ref-hint"
                  />
                  <datalist id="model-ref-suggestions">
                    {(engine?.suggestions ?? []).map((s) => (
                      <option key={s} value={s} />
                    ))}
                  </datalist>
                  {shown.model ? (
                    <p
                      id="model-ref-hint"
                      role="alert"
                      className="text-xs text-destructive"
                    >
                      {t('deploy.errors.model')}
                    </p>
                  ) : (
                    <FieldHint id="model-ref-hint">
                      {t('deploy.fields.modelHint')}
                    </FieldHint>
                  )}
                  <div
                    role="group"
                    aria-label={t('deploy.fields.modelSuggestions')}
                    className="flex flex-wrap gap-1.5 pt-1"
                  >
                    {(engine?.suggestions ?? []).map((s) => (
                      <button
                        key={s}
                        type="button"
                        onClick={() => {
                          set('model', s)
                        }}
                        className="rounded-md border border-border bg-muted/40 px-2 py-0.5 font-mono text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
                      >
                        {s}
                      </button>
                    ))}
                  </div>
                </Field>
              </div>
              <div className="space-y-3">
                <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  {t('deploy.sections.placement')}
                </h3>
                <Field>
                  <FieldLabel htmlFor="model-node">
                    {t('deploy.fields.node')}
                  </FieldLabel>
                  <Select
                    value={form.node}
                    onValueChange={(v) => {
                      if (v) set('node', v)
                    }}
                  >
                    <SelectTrigger id="model-node" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={LOCAL_NODE}>
                        {t('deploy.fields.localNode')}
                      </SelectItem>
                      {usableNodes
                        .filter((n) => !n.is_local)
                        .map((n) => (
                          <SelectItem key={n.node_id} value={n.node_id}>
                            {nodeOptionText(n)}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                </Field>
                <ModelPreflightPanel
                  engine={form.engine}
                  model={form.model}
                  node={form.node}
                  hfToken={form.hfToken}
                  gpus={form.gpus}
                  onPickModel={(m) => {
                    set('model', m)
                  }}
                />
                <ModelFitPanel form={form} />
              </div>
            </div>
            <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
              <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 rounded-lg border border-border px-3 py-2 text-left text-sm font-medium text-foreground transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none">
                <span>
                  {t('deploy.sections.advanced')}
                  <span className="block text-xs font-normal text-muted-foreground">
                    {t('deploy.sections.advancedHint')}
                  </span>
                </span>
                <ChevronDownGlassIcon
                  className={`size-4.5 shrink-0 text-muted-foreground transition-transform ${advancedOpen ? 'rotate-180' : ''}`}
                  aria-hidden="true"
                />
              </CollapsibleTrigger>
              <CollapsiblePanel>
                <div className="pt-3">
                  <DeployAdvancedFields form={form} errors={shown} set={set} />
                </div>
              </CollapsiblePanel>
            </Collapsible>
            {apiError ? (
              <p role="alert" className="text-sm text-destructive">
                {apiError}
              </p>
            ) : null}
            <div className="sticky bottom-0 -mx-4 -mb-4 flex flex-wrap items-center justify-between gap-3 border-t border-border bg-popover px-4 py-3">
              <p className="min-w-0 flex-1 text-xs text-muted-foreground">
                {summary}
              </p>
              <div className="flex shrink-0 gap-2">
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    handleOpenChange(false)
                  }}
                >
                  {t('deploy.actions.cancel')}
                </Button>
                <Button type="submit" disabled={create.isPending}>
                  {create.isPending
                    ? t('deploy.actions.deploying')
                    : t('deploy.actions.deploy')}
                </Button>
              </div>
            </div>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
