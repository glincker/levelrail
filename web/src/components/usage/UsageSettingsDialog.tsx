import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useSaveUsageSettings } from '../../queries/usage'
import type {
  UsageBasis,
  UsageMode,
  UsageSettings,
  UsageSettingsValues,
} from '../../types/usage'

interface NodeDraft {
  node_id: string
  name: string
  monthly_price: string
  vcpus: string
  memory_gib: string
  detectedVcpus: number
  detectedMemory: string
}

function num(v: string): number {
  const n = Number(v)
  return Number.isFinite(n) && n >= 0 ? n : 0
}

function NumberField({
  label,
  value,
  onChange,
  hint,
  step = '0.01',
}: {
  label: string
  value: string
  onChange: (v: string) => void
  hint?: string
  step?: string
}) {
  const id = useId()
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type="number"
        inputMode="decimal"
        min="0"
        step={step}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
    </div>
  )
}

function Choice<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: T
  options: { value: T; label: string }[]
  onChange: (v: T) => void
}) {
  return (
    <div className="space-y-1">
      <span className="text-sm font-medium text-foreground">{label}</span>
      <div className="flex flex-wrap gap-1" role="group" aria-label={label}>
        {options.map((o) => (
          <Button
            key={o.value}
            size="sm"
            variant={value === o.value ? 'secondary' : 'ghost'}
            aria-pressed={value === o.value}
            onClick={() => onChange(o.value)}
          >
            {o.label}
          </Button>
        ))}
      </div>
    </div>
  )
}

function toDrafts(data: UsageSettings): NodeDraft[] {
  return data.nodes.map((n) => ({
    node_id: n.node_id,
    name: n.name,
    monthly_price: n.monthly_price ? String(n.monthly_price) : '',
    vcpus: n.vcpus ? String(n.vcpus) : '',
    memory_gib: n.memory_gib ? String(n.memory_gib) : '',
    detectedVcpus: n.detected_vcpus,
    detectedMemory: n.detected_memory_gib.toFixed(1),
  }))
}

// Prices and allocation form. Mounted fresh per open so its draft always
// starts from the saved values.
export function UsageSettingsDialog({
  open,
  onOpenChange,
  data,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  data: UsageSettings
}) {
  const { t } = useTranslation('usage')
  const save = useSaveUsageSettings()
  const s = data.settings
  const [mode, setMode] = useState<UsageMode>(s.mode)
  const [basis, setBasis] = useState<UsageBasis>(s.basis)
  const [currency, setCurrency] = useState(s.currency)
  const [cpu, setCpu] = useState(String(s.cpu_core_hour))
  const [mem, setMem] = useState(String(s.mem_gib_hour))
  const [storage, setStorage] = useState(String(s.storage_gib_month))
  const [egress, setEgress] = useState(String(s.egress_gib))
  const [idle, setIdle] = useState(String(s.idle_spread_pct))
  const [weight, setWeight] = useState(String(s.cpu_weight_pct))
  const [nodes, setNodes] = useState<NodeDraft[]>(() => toDrafts(data))
  const [reprice, setReprice] = useState(false)
  const repriceId = useId()
  const currencyId = useId()
  const presetId = useId()

  function patchNode(id: string, patch: Partial<NodeDraft>) {
    setNodes((ns) => ns.map((n) => (n.node_id === id ? { ...n, ...patch } : n)))
  }

  function applyPreset(key: string) {
    const p = data.presets.find((x) => x.key === key)
    if (!p) return
    setCpu(String(Number(p.cpu_core_hour.toFixed(5))))
    setMem(String(Number(p.mem_gib_hour.toFixed(5))))
  }

  function submit() {
    const settings: UsageSettingsValues = {
      mode,
      basis,
      currency: currency.trim().toUpperCase() || 'USD',
      cpu_core_hour: num(cpu),
      mem_gib_hour: num(mem),
      storage_gib_month: num(storage),
      egress_gib: num(egress),
      idle_spread_pct: Math.min(100, num(idle)),
      cpu_weight_pct: Math.min(100, Math.max(1, num(weight))),
    }
    save.mutate(
      {
        settings,
        nodes: nodes
          .filter((n) => num(n.monthly_price) > 0)
          .map((n) => ({
            node_id: n.node_id,
            monthly_price: num(n.monthly_price),
            vcpus: num(n.vcpus),
            memory_gib: num(n.memory_gib),
          })),
        reprice_month: reprice,
      },
      {
        onSuccess: () => {
          toast.add({ type: 'success', title: t('settings.saved') })
          onOpenChange(false)
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('settings.title')}</DialogTitle>
          <DialogDescription>{t('settings.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-5">
          <Choice
            label={t('settings.mode')}
            value={mode}
            onChange={setMode}
            options={[
              { value: 'node', label: t('settings.modeNode') },
              { value: 'unit', label: t('settings.modeUnit') },
            ]}
          />
          <p className="text-xs text-muted-foreground">
            {mode === 'node'
              ? t('settings.modeNodeHint')
              : t('settings.modeUnitHint')}
          </p>
          <div className="space-y-1">
            <Label htmlFor={currencyId}>{t('settings.currency')}</Label>
            <Input
              id={currencyId}
              value={currency}
              maxLength={8}
              onChange={(e) => setCurrency(e.target.value)}
            />
          </div>
          {mode === 'node' ? (
            <fieldset className="space-y-3">
              <legend className="text-sm font-medium text-foreground">
                {t('settings.nodes')}
              </legend>
              {nodes.map((n) => (
                <div
                  key={n.node_id}
                  className="space-y-2 rounded-lg border border-border p-3"
                >
                  <p className="text-sm font-medium">{n.name}</p>
                  <div className="grid gap-3 sm:grid-cols-3">
                    <NumberField
                      label={t('settings.nodeMonthly')}
                      value={n.monthly_price}
                      onChange={(v) =>
                        patchNode(n.node_id, { monthly_price: v })
                      }
                    />
                    <NumberField
                      label={t('settings.nodeVcpus')}
                      value={n.vcpus}
                      step="1"
                      onChange={(v) => patchNode(n.node_id, { vcpus: v })}
                    />
                    <NumberField
                      label={t('settings.nodeMemory')}
                      value={n.memory_gib}
                      step="0.5"
                      onChange={(v) => patchNode(n.node_id, { memory_gib: v })}
                    />
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {t('settings.detected', {
                      vcpus: n.detectedVcpus,
                      memory: n.detectedMemory,
                    })}
                  </p>
                </div>
              ))}
              <NumberField
                label={t('settings.weight')}
                hint={t('settings.weightHint')}
                value={weight}
                step="1"
                onChange={setWeight}
              />
            </fieldset>
          ) : (
            <fieldset className="space-y-3">
              <div className="space-y-1">
                <Label htmlFor={presetId}>{t('settings.preset')}</Label>
                <div id={presetId} className="flex flex-wrap gap-1">
                  {data.presets.map((p) => (
                    <Button
                      key={p.key}
                      size="sm"
                      variant="outline"
                      onClick={() => applyPreset(p.key)}
                    >
                      {p.label}
                    </Button>
                  ))}
                </div>
                <p className="text-xs text-muted-foreground">
                  {t('settings.presetHint')}
                </p>
              </div>
              <div className="grid gap-3 sm:grid-cols-2">
                <NumberField
                  label={t('settings.cpuHour')}
                  value={cpu}
                  step="0.0001"
                  onChange={setCpu}
                />
                <NumberField
                  label={t('settings.memHour')}
                  value={mem}
                  step="0.0001"
                  onChange={setMem}
                />
              </div>
            </fieldset>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <NumberField
              label={t('settings.storage')}
              value={storage}
              onChange={setStorage}
            />
            <NumberField
              label={t('settings.egress')}
              value={egress}
              onChange={setEgress}
            />
          </div>
          <NumberField
            label={t('settings.idle')}
            hint={
              mode === 'node'
                ? t('settings.idleHintNode')
                : t('settings.idleHintUnit')
            }
            value={idle}
            step="1"
            onChange={setIdle}
          />
          <Choice
            label={t('settings.basis')}
            value={basis}
            onChange={setBasis}
            options={[
              { value: 'avg', label: t('settings.basisAvg') },
              { value: 'p50', label: t('settings.basisP50') },
            ]}
          />
          <div className="flex items-start gap-2">
            <Switch
              id={repriceId}
              checked={reprice}
              onCheckedChange={setReprice}
            />
            <div className="space-y-0.5">
              <Label htmlFor={repriceId}>{t('settings.reprice')}</Label>
              <p className="text-xs text-muted-foreground">
                {t('settings.repriceHint')}
              </p>
            </div>
          </div>
          {save.isError ? (
            <p className="text-sm text-destructive" role="alert">
              {t('settings.error', { message: save.error.message })}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('settings.cancel')}
          </Button>
          <Button onClick={submit} disabled={save.isPending}>
            {save.isPending ? t('settings.saving') : t('settings.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
