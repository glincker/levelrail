import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useCreateAlertRule } from '../queries/alerts'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import type { CreateAlertRuleRequest, LogRuleConfig } from '../types/alerts'

export interface LogAlertDefaults {
  name?: string
  query?: string
  regex?: string
  level?: string
  fields?: string[]
}

const WINDOWS = [
  { value: '300', label: '5m' },
  { value: '900', label: '15m' },
  { value: '3600', label: '1h' },
]
const COOLDOWNS = [
  { value: '0', label: 'off' },
  { value: '900', label: '15m' },
  { value: '3600', label: '1h' },
]

// Creates a log_pattern or log_level_rate rule from the current search or
// from one error signature ("alert when this matches more than N times in
// 5 minutes"). Evaluated server-side from the node-local log store.
export function CreateLogAlertDialog({
  appName,
  open,
  onOpenChange,
  defaults,
}: {
  appName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  defaults: LogAlertDefaults
}) {
  const { t } = useTranslation('observability')
  const create = useCreateAlertRule(appName)
  const channels = useNotificationChannelsOptional()
  const [mode, setMode] = useState<'count' | 'rate'>('count')
  const [name, setName] = useState(defaults.name ?? '')
  const [count, setCount] = useState('10')
  const [perMinute, setPerMinute] = useState('5')
  const [windowSeconds, setWindowSeconds] = useState('300')
  const [cooldown, setCooldown] = useState('900')
  const [channelId, setChannelId] = useState('')

  const threshold = Number(mode === 'count' ? count : perMinute)
  const valid =
    name.trim() !== '' && Number.isFinite(threshold) && threshold > 0

  function submit() {
    const rule: LogRuleConfig = {
      query: defaults.query || undefined,
      regex: defaults.regex || undefined,
      level: mode === 'rate' ? (defaults.level ?? 'error') : defaults.level,
      fields: defaults.fields?.length ? defaults.fields : undefined,
      window_seconds: Number(windowSeconds),
      cooldown_seconds: Number(cooldown) || undefined,
    }
    if (mode === 'count') {
      rule.count = threshold
    } else {
      rule.per_minute = threshold
    }
    const req: CreateAlertRuleRequest = {
      name: name.trim(),
      kind: mode === 'count' ? 'log_pattern' : 'log_level_rate',
      log_rule: rule,
      channel_id: channelId || undefined,
      enabled: true,
    }
    create.mutate(req, {
      onSuccess: () => {
        onOpenChange(false)
        toast.add({
          title: t('explorer.alertCreated', { name: req.name }),
          type: 'success',
        })
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('explorer.alertTitle')}</DialogTitle>
          <DialogDescription>
            {t('explorer.alertDescription')}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <p className="rounded bg-muted px-2 py-1 font-mono text-xs break-words text-muted-foreground">
            {defaults.regex ||
              defaults.query ||
              (defaults.fields ?? []).join(' ') ||
              t('explorer.alertAnyLine', { level: defaults.level ?? 'error' })}
          </p>
          <label className="block text-sm">
            {t('explorer.alertName')}
            <Input
              value={name}
              onChange={(e) => {
                setName(e.target.value)
              }}
              className="mt-1"
            />
          </label>
          <div className="flex flex-wrap gap-2">
            <Select
              value={mode}
              onValueChange={(next) => {
                setMode(next as 'count' | 'rate')
              }}
            >
              <SelectTrigger
                className="w-44"
                aria-label={t('explorer.alertMode')}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="count">{t('explorer.modeCount')}</SelectItem>
                <SelectItem value="rate">{t('explorer.modeRate')}</SelectItem>
              </SelectContent>
            </Select>
            <Input
              type="number"
              min={1}
              value={mode === 'count' ? count : perMinute}
              onChange={(e) => {
                if (mode === 'count') {
                  setCount(e.target.value)
                } else {
                  setPerMinute(e.target.value)
                }
              }}
              className="w-24"
              aria-label={t('explorer.alertThreshold')}
            />
            <Select
              value={windowSeconds}
              onValueChange={(v) => {
                if (v) setWindowSeconds(v)
              }}
            >
              <SelectTrigger
                className="w-28"
                aria-label={t('explorer.alertWindow')}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {WINDOWS.map((w) => (
                  <SelectItem key={w.value} value={w.value}>
                    {t('explorer.inWindow', { window: w.label })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="text-muted-foreground">
              {t('explorer.cooldown')}
            </span>
            <Select
              value={cooldown}
              onValueChange={(v) => {
                if (v) setCooldown(v)
              }}
            >
              <SelectTrigger
                className="w-24"
                aria-label={t('explorer.cooldown')}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {COOLDOWNS.map((c) => (
                  <SelectItem key={c.value} value={c.value}>
                    {c.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={channelId}
              onValueChange={(v) => {
                setChannelId(v ?? '')
              }}
            >
              <SelectTrigger
                className="w-48"
                aria-label={t('explorer.channel')}
              >
                <SelectValue placeholder={t('explorer.channelNone')} />
              </SelectTrigger>
              <SelectContent>
                {(Array.isArray(channels.data) ? channels.data : []).map(
                  (c) => (
                    <SelectItem key={c.id} value={c.id}>
                      {c.name}
                    </SelectItem>
                  ),
                )}
              </SelectContent>
            </Select>
          </div>
          {create.error ? (
            <p className="text-sm text-destructive">{create.error.message}</p>
          ) : null}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              onOpenChange(false)
            }}
          >
            {t('explorer.cancel')}
          </Button>
          <Button
            type="button"
            disabled={!valid || create.isPending}
            onClick={submit}
          >
            {t('explorer.createAlert')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
