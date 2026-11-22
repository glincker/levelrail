import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilGlassIcon } from '@thesvg/ui-react'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useUpdateAlertRule } from '../queries/alerts'
import type { AlertRule, CreateAlertRuleRequest } from '../types/alerts'

const DEFAULT_WINDOW_SECONDS = 300

function minutesOf(seconds: number | undefined, fallback: number): string {
  return String(Math.round((seconds ?? fallback) / 60))
}

// Edits only the numbers that tune a log rule; the query, filters and
// delivery settings are sent back unchanged so PUT does not drop them.
export function EditLogRuleDialog({
  appName,
  rule,
}: {
  appName: string
  rule: AlertRule
}) {
  const { t } = useTranslation('logRuleEdit')
  const cfg = rule.log_rule
  const isRate = rule.kind === 'log_level_rate'
  const [open, setOpen] = useState(false)
  const [threshold, setThreshold] = useState(
    String(isRate ? (cfg?.per_minute ?? 0) : (cfg?.count ?? 0)),
  )
  const [windowMin, setWindowMin] = useState(
    minutesOf(cfg?.window_seconds, DEFAULT_WINDOW_SECONDS),
  )
  const [cooldownMin, setCooldownMin] = useState(
    minutesOf(cfg?.cooldown_seconds, 0),
  )
  const updateRule = useUpdateAlertRule(appName)

  if (!cfg) {
    return null
  }

  function submit() {
    if (!cfg) return
    const next = {
      ...cfg,
      window_seconds: Math.round(Number(windowMin) * 60),
      cooldown_seconds: Math.round(Number(cooldownMin) * 60),
      ...(isRate
        ? { per_minute: Number(threshold) }
        : { count: Math.round(Number(threshold)) }),
    }
    const req: CreateAlertRuleRequest = {
      name: rule.name,
      kind: rule.kind,
      channel_id: rule.channel_id,
      notify_url: rule.notify_url,
      notify_kind: rule.notify_kind,
      enabled: rule.enabled,
      severity: rule.severity,
      labels: rule.labels,
      consecutive_failures: rule.consecutive_failures,
      flap_threshold: rule.flap_threshold,
      flap_window: rule.flap_window,
      log_rule: next,
    }
    updateRule.mutate(
      { id: rule.id, req },
      {
        onSuccess: () => {
          setOpen(false)
          toast.add({
            title: t('updated', { name: rule.name }),
            type: 'success',
          })
        },
      },
    )
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) updateRule.reset()
      }}
    >
      <SheetTrigger render={<Button variant="outline" size="sm" />}>
        <PencilGlassIcon className="size-5" aria-hidden="true" />
        {t('trigger')}
      </SheetTrigger>
      <SheetContent className="overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{t('title')}</SheetTitle>
          <SheetDescription>{t('description')}</SheetDescription>
        </SheetHeader>
        <form
          className="space-y-4 px-4 pb-4"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <Field>
            <FieldLabel htmlFor="edit-log-rule-threshold">
              {isRate ? t('perMinute') : t('count')}
            </FieldLabel>
            <Input
              id="edit-log-rule-threshold"
              type="number"
              min="0"
              step={isRate ? 'any' : '1'}
              value={threshold}
              onChange={(e) => setThreshold(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="edit-log-rule-window">
              {t('windowMinutes')}
            </FieldLabel>
            <Input
              id="edit-log-rule-window"
              type="number"
              min="1"
              step="1"
              value={windowMin}
              onChange={(e) => setWindowMin(e.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="edit-log-rule-cooldown">
              {t('cooldownMinutes')}
            </FieldLabel>
            <Input
              id="edit-log-rule-cooldown"
              type="number"
              min="0"
              step="1"
              value={cooldownMin}
              onChange={(e) => setCooldownMin(e.target.value)}
            />
          </Field>
          {updateRule.isError ? (
            <p className="text-sm text-destructive">
              {updateRule.error.message}
            </p>
          ) : null}
          <SheetFooter>
            <Button type="submit" disabled={updateRule.isPending}>
              {updateRule.isPending ? t('saving') : t('save')}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  )
}
