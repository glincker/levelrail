import { useState } from 'react'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
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
import { MODEL_ENGINES } from '../lib/models'
import {
  INITIAL_DEPLOY_FORM,
  LOCAL_NODE,
  buildCreateRequest,
  validateDeployForm,
  type DeployFormState,
} from '../lib/modelDeployForm'
import { useCreateModel, useGpuNodes } from '../queries/models'
import { ModelPreflightPanel } from './ModelPreflightPanel'
import type { CreateModelResponse } from '../types/models'

export function DeployModelDialog({
  onDeployed,
}: {
  onDeployed: (created: CreateModelResponse) => void
}) {
  const [open, setOpen] = useState(false)
  const [form, setForm] = useState<DeployFormState>(INITIAL_DEPLOY_FORM)
  const [problem, setProblem] = useState<string | null>(null)
  const create = useCreateModel()
  const gpuNodes = useGpuNodes()
  const engine = MODEL_ENGINES.find((e) => e.id === form.engine)

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
      setProblem(null)
      create.reset()
    }
  }

  function submit() {
    const invalid = validateDeployForm(form)
    setProblem(invalid)
    if (invalid) return
    create.mutate(buildCreateRequest(form), {
      onSuccess: (created) => {
        toast.add({
          title: `Deploying "${created.name}".`,
          description: 'The engine is downloading and loading the model.',
          type: 'success',
        })
        handleOpenChange(false)
        onDeployed(created)
      },
    })
  }

  const usableNodes = (gpuNodes.data ?? []).filter((n) => n.present)
  const errorText = problem ?? (create.isError ? create.error.message : null)

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button size="sm" />}>
        <PlusIcon aria-hidden="true" />
        Deploy model
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Deploy a model</DialogTitle>
          <DialogDescription>
            Runs an inference engine on a GPU node and exposes an
            OpenAI-compatible endpoint protected by an API key.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <Field>
            <FieldLabel htmlFor="model-name">Name</FieldLabel>
            <Input
              id="model-name"
              value={form.name}
              onChange={(e) => {
                set('name', e.target.value)
              }}
              placeholder="chat"
              autoComplete="off"
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="model-engine">Engine</FieldLabel>
              <Select
                value={form.engine}
                onValueChange={(v) => {
                  if (v) set('engine', v)
                }}
              >
                <SelectTrigger id="model-engine" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {MODEL_ENGINES.map((e) => (
                    <SelectItem key={e.id} value={e.id}>
                      {e.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel htmlFor="model-node">Node</FieldLabel>
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
                  <SelectItem value={LOCAL_NODE}>This host</SelectItem>
                  {usableNodes
                    .filter((n) => !n.is_local)
                    .map((n) => (
                      <SelectItem key={n.node_id} value={n.node_id}>
                        {n.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="model-ref">Model</FieldLabel>
            <Input
              id="model-ref"
              value={form.model}
              onChange={(e) => {
                set('model', e.target.value)
              }}
              placeholder={engine?.refHint}
              autoComplete="off"
              className="font-mono"
            />
          </Field>
          <ModelPreflightPanel
            engine={form.engine}
            model={form.model}
            node={form.node}
            hfToken={form.hfToken}
            onPickModel={(m) => {
              set('model', m)
            }}
          />
          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="model-gpus">GPUs</FieldLabel>
              <Input
                id="model-gpus"
                value={form.gpus}
                onChange={(e) => {
                  set('gpus', e.target.value)
                }}
                placeholder="all"
              />
              <FieldHint>&ldquo;all&rdquo; or a number.</FieldHint>
            </Field>
            <Field>
              <FieldLabel htmlFor="model-context">Context length</FieldLabel>
              <Input
                id="model-context"
                value={form.context}
                onChange={(e) => {
                  set('context', e.target.value)
                }}
                placeholder="engine default"
                inputMode="numeric"
              />
            </Field>
          </div>
          {form.engine === 'vllm' ? (
            <Field>
              <FieldLabel htmlFor="model-quant">Quantization</FieldLabel>
              <Input
                id="model-quant"
                value={form.quantization}
                onChange={(e) => {
                  set('quantization', e.target.value)
                }}
                placeholder="awq, gptq, fp8 (optional)"
              />
            </Field>
          ) : null}
          <Field>
            <FieldLabel htmlFor="model-domain">Domain</FieldLabel>
            <Input
              id="model-domain"
              value={form.domain}
              onChange={(e) => {
                set('domain', e.target.value)
              }}
              placeholder="llm.example.com (optional)"
              autoComplete="off"
            />
          </Field>
          {form.engine !== 'ollama' ? (
            <Field>
              <FieldLabel htmlFor="model-hf-token">
                HuggingFace token
              </FieldLabel>
              <Input
                id="model-hf-token"
                type="password"
                value={form.hfToken}
                onChange={(e) => {
                  set('hfToken', e.target.value)
                }}
                placeholder="hf_... (only for gated models)"
                autoComplete="off"
              />
              <FieldHint>Stored encrypted, never shown again.</FieldHint>
            </Field>
          ) : null}
          {errorText ? (
            <p role="alert" className="text-sm text-destructive">
              {errorText}
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
            <Button type="submit" disabled={create.isPending}>
              {create.isPending ? 'Deploying...' : 'Deploy'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
