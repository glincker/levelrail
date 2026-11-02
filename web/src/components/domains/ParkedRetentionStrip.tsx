import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
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
import { toast } from '@/components/ui/toast'
import {
  clampRetentionDays,
  estimateRetention,
  RETENTION_MAX_DAYS,
  RETENTION_PRESETS,
} from '../../lib/parkedRetention'
import {
  parkedRetentionKeys,
  RETENTION_SETTING_KEY,
  useParkedRetention,
  type ParkedRetention,
} from '../../queries/parkedRetention'
import type { ParkedDomain } from '../../queries/parkedDomains'
import { useSetRuntimeSetting } from '../../queries/runtimeSettings'

const DEFAULT_DAYS = 30

function formatNext(iso?: string): string {
  if (!iso) return ''
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString()
}

interface PickerProps {
  retention: ParkedRetention
  parked: readonly ParkedDomain[]
  days: number
  onDays: (n: number) => void
}

// Exact server data when the days match the saved setting, else an estimate.
function RetentionPreview({ retention, parked, days, onDays }: PickerProps) {
  const { t } = useTranslation('domains')
  const customId = useId()
  const [custom, setCustom] = useState(
    (RETENTION_PRESETS as readonly number[]).includes(days),
  )
  const [customText, setCustomText] = useState(String(days))
  const exact =
    retention.retention_days > 0 && days === retention.retention_days
  const est = estimateRetention(parked, days, retention.batch_limit, new Date())
  const list = exact ? retention.would_delete : est.firstRun
  const total = exact ? retention.total_eligible : est.total
  return (
    <div className="space-y-2">
      <div
        role="radiogroup"
        aria-label={t('retention.daysLabel')}
        className="flex flex-wrap items-center gap-1.5"
      >
        {RETENTION_PRESETS.map((n) => (
          <Button
            key={n}
            type="button"
            size="sm"
            role="radio"
            aria-checked={!custom && days === n}
            variant={!custom && days === n ? 'default' : 'outline'}
            onClick={() => {
              setCustom(false)
              onDays(n)
            }}
          >
            {t('retention.days', { count: n })}
          </Button>
        ))}
        <Button
          type="button"
          size="sm"
          role="radio"
          aria-checked={custom}
          variant={custom ? 'default' : 'outline'}
          onClick={() => {
            setCustom(true)
          }}
        >
          {t('retention.custom')}
        </Button>
        {custom ? (
          <>
            <Label htmlFor={customId} className="sr-only">
              {t('retention.customLabel')}
            </Label>
            <Input
              id={customId}
              type="number"
              inputMode="numeric"
              min={1}
              max={RETENTION_MAX_DAYS}
              className="h-8 w-24"
              value={customText}
              onChange={(e) => {
                setCustomText(e.target.value)
                const n = clampRetentionDays(Number(e.target.value))
                if (n > 0) onDays(n)
              }}
            />
          </>
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">
        {exact ? t('retention.exact') : t('retention.estimate')}
      </p>
      <p className="text-sm" aria-live="polite">
        {t('retention.dueNow', { count: total })}
      </p>
      {list.length > 0 ? (
        <ul className="max-h-40 divide-y divide-border overflow-auto rounded-md border border-border text-sm">
          {list.map((c) => (
            <li key={c.domain} className="flex items-center gap-2 px-2 py-1">
              <span className="min-w-0 flex-1 truncate font-mono">
                {c.domain}
              </span>
              <span className="text-xs text-muted-foreground">
                {t('retention.age', { count: c.age_days })}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      {total > list.length ? (
        <p className="text-xs text-muted-foreground">
          {t('retention.batchNote', { limit: retention.batch_limit })}
        </p>
      ) : null}
    </div>
  )
}

type Mode = 'preview' | 'enable' | 'disable'

export function ParkedRetentionStrip({
  parked,
}: {
  parked: readonly ParkedDomain[]
}) {
  const { t } = useTranslation('domains')
  const { data: retention } = useParkedRetention()
  const save = useSetRuntimeSetting()
  const queryClient = useQueryClient()
  const [mode, setMode] = useState<Mode | null>(null)
  const [days, setDays] = useState(DEFAULT_DAYS)
  const [error, setError] = useState<string | null>(null)
  if (!retention) return null
  const on = retention.retention_days > 0

  function open(next: Mode) {
    setDays(on ? retention!.retention_days : DEFAULT_DAYS)
    setError(null)
    setMode(next)
  }

  function apply(value: number) {
    setError(null)
    save.mutate(
      { key: RETENTION_SETTING_KEY, value: String(value) },
      {
        onSuccess: () => {
          void queryClient.invalidateQueries({
            queryKey: parkedRetentionKeys.all,
          })
          toast.add({
            title:
              value > 0
                ? t('retention.savedOn', { count: value })
                : t('retention.savedOff'),
            type: 'success',
          })
          setMode(null)
        },
        onError: (e) => {
          setError(e instanceof Error ? e.message : t('retention.saveFailed'))
        },
      },
    )
  }

  const est = estimateRetention(parked, days, retention.batch_limit, new Date())
  const exact = on && days === retention.retention_days
  const dueCount = exact ? retention.total_eligible : est.total
  const firstRun = Math.min(dueCount, retention.batch_limit)

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
      <span className="font-medium">{t('retention.title')}</span>
      <span className="min-w-0 flex-1 text-xs text-muted-foreground">
        {on
          ? t('retention.summaryOn', {
              days: retention.retention_days,
              due: retention.total_eligible,
              next: formatNext(retention.next_run),
            })
          : t('retention.summaryOff')}
      </span>
      <Button
        size="sm"
        variant="outline"
        onClick={() => {
          open('preview')
        }}
      >
        {t('retention.preview')}
      </Button>
      <Button
        size="sm"
        variant={on ? 'outline' : 'default'}
        onClick={() => {
          open(on ? 'disable' : 'enable')
        }}
      >
        {on ? t('retention.turnOff') : t('retention.turnOn')}
      </Button>
      {mode === 'preview' ? (
        <Dialog
          open
          onOpenChange={(o) => {
            if (!o) setMode(null)
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('retention.previewTitle')}</DialogTitle>
              <DialogDescription>
                {t('retention.previewBody')}
              </DialogDescription>
            </DialogHeader>
            <RetentionPreview
              retention={retention}
              parked={parked}
              days={days}
              onDays={setDays}
            />
            <DialogFooter>
              <Button
                variant="outline"
                onClick={() => {
                  setMode(null)
                }}
              >
                {t('retention.close')}
              </Button>
              {!on ? (
                <Button
                  onClick={() => {
                    setMode('enable')
                  }}
                >
                  {t('retention.turnOnDays', { count: days })}
                </Button>
              ) : null}
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ) : null}
      {mode === 'enable' ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => {
            if (!o) setMode(null)
          }}
          title={t('retention.confirmTitle', { count: days })}
          description={t('retention.confirmBody', {
            due: dueCount,
            first: firstRun,
            limit: retention.batch_limit,
          })}
          consequences={[
            t('retention.consequenceDns'),
            t('retention.consequenceBatch', { limit: retention.batch_limit }),
          ]}
          confirmLabel={t('retention.turnOn')}
          pending={save.isPending}
          error={error}
          focusCancel
          onConfirm={() => {
            apply(days)
          }}
        >
          <RetentionPreview
            retention={retention}
            parked={parked}
            days={days}
            onDays={setDays}
          />
        </ConfirmDialog>
      ) : null}
      {mode === 'disable' ? (
        <ConfirmDialog
          open
          onOpenChange={(o) => {
            if (!o) setMode(null)
          }}
          title={t('retention.offTitle')}
          description={t('retention.offBody')}
          confirmLabel={t('retention.turnOff')}
          pending={save.isPending}
          error={error}
          onConfirm={() => {
            apply(0)
          }}
        />
      ) : null}
    </div>
  )
}
