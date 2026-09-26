import { useState } from 'react'
import { SlidersHorizontalIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useUpdateAlertRule } from '../queries/alerts'
import type { AlertRule, CreateAlertRuleRequest } from '../types/alerts'
import type { Severity } from '../types/alertNoise'

const SEVERITIES: Severity[] = ['info', 'warning', 'critical']
const GO_DURATION = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/

// A PUT replaces the whole rule, so the request restates every editable
// field from the rule as listed; only the noise settings change here.
function ruleToRequest(rule: AlertRule): CreateAlertRuleRequest {
  return {
    name: rule.name,
    kind: rule.kind,
    metric: rule.metric,
    comparator: rule.comparator,
    threshold: rule.threshold,
    for_duration: rule.for_duration,
    restart_count_threshold: rule.restart_count_threshold,
    restart_window: rule.restart_window,
    scheduled_task_id: rule.scheduled_task_id,
    backup_resource_kind: rule.backup_resource_kind,
    backup_database_name: rule.backup_database_name,
    backup_service_name: rule.backup_service_name,
    backup_volume_name: rule.backup_volume_name,
    channel_id: rule.channel_id,
    notify_url: rule.channel_id ? undefined : rule.notify_url,
    notify_kind: rule.channel_id ? undefined : rule.notify_kind,
    enabled: rule.enabled,
  }
}

function parseLabels(raw: string): Record<string, string> | undefined {
  const out: Record<string, string> = {}
  for (const part of raw.split(',')) {
    const [k, ...rest] = part.split('=')
    const key = k?.trim()
    if (key && rest.length > 0) out[key] = rest.join('=').trim()
  }
  return Object.keys(out).length > 0 ? out : undefined
}

// Per-rule noise control: severity, how many consecutive failing ticks
// before firing, and when a rule counts as flapping. Blank numeric
// fields fall back to the control plane defaults.
export function RuleNoiseDialog({
  appName,
  rule,
}: {
  appName: string
  rule: AlertRule
}) {
  const [open, setOpen] = useState(false)
  const [severity, setSeverity] = useState<Severity>(rule.severity ?? 'warning')
  const [consecutive, setConsecutive] = useState(
    String(rule.consecutive_failures ?? ''),
  )
  const [flapThreshold, setFlapThreshold] = useState(
    String(rule.flap_threshold ?? ''),
  )
  const [flapWindow, setFlapWindow] = useState(rule.flap_window ?? '')
  const [labels, setLabels] = useState(
    Object.entries(rule.labels ?? {})
      .map(([k, v]) => `${k}=${v}`)
      .join(', '),
  )
  const [formError, setFormError] = useState('')
  const update = useUpdateAlertRule(appName)

  function submit() {
    const consecutiveN = consecutive === '' ? 0 : Number(consecutive)
    const flapN = flapThreshold === '' ? 0 : Number(flapThreshold)
    if (
      !Number.isInteger(consecutiveN) ||
      !Number.isInteger(flapN) ||
      consecutiveN < 0 ||
      flapN < 0
    ) {
      setFormError('Counts must be whole numbers, or blank for the default.')
      return
    }
    if (flapWindow !== '' && !GO_DURATION.test(flapWindow)) {
      setFormError('Flap window must look like 30m or 1h.')
      return
    }
    setFormError('')
    update.mutate(
      {
        id: rule.id,
        req: {
          ...ruleToRequest(rule),
          severity,
          labels: parseLabels(labels),
          consecutive_failures: consecutiveN || undefined,
          flap_threshold: flapN || undefined,
          flap_window: flapWindow || undefined,
        },
      },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: `Noise settings for "${rule.name}" saved.`,
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger
        render={
          <Button
            variant="outline"
            size="sm"
            aria-label={`Noise settings for ${rule.name}`}
          />
        }
      >
        <SlidersHorizontalIcon className="size-3.5" aria-hidden="true" />
        Noise
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            Noise settings for &ldquo;{rule.name}&rdquo;
          </DialogTitle>
          <DialogDescription>
            Hold a rule back until its condition has held for several checks in
            a row, and mute it automatically when it fires and resolves over and
            over. Blank counts use the control plane defaults.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Field>
            <FieldLabel htmlFor="noise-severity">Severity</FieldLabel>
            <Select
              value={severity}
              onValueChange={(v) => {
                setSeverity(v as Severity)
              }}
            >
              <SelectTrigger id="noise-severity" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SEVERITIES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="noise-consecutive">
              Consecutive failing checks before firing
            </FieldLabel>
            <Input
              id="noise-consecutive"
              inputMode="numeric"
              placeholder="default"
              value={consecutive}
              onChange={(e) => {
                setConsecutive(e.target.value)
              }}
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="noise-flap">
                Flapping after N fires
              </FieldLabel>
              <Input
                id="noise-flap"
                inputMode="numeric"
                placeholder="default"
                value={flapThreshold}
                onChange={(e) => {
                  setFlapThreshold(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="noise-flap-window">Within</FieldLabel>
              <Input
                id="noise-flap-window"
                placeholder="default, e.g. 30m"
                value={flapWindow}
                onChange={(e) => {
                  setFlapWindow(e.target.value)
                }}
              />
            </Field>
          </div>
          <Field>
            <FieldLabel htmlFor="noise-labels">Labels</FieldLabel>
            <Input
              id="noise-labels"
              placeholder="team=core, env=prod"
              value={labels}
              onChange={(e) => {
                setLabels(e.target.value)
              }}
            />
          </Field>
          {formError || update.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {formError || update.error?.message}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setOpen(false)
            }}
          >
            Cancel
          </Button>
          <Button type="button" disabled={update.isPending} onClick={submit}>
            {update.isPending ? 'Saving...' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
