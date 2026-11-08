import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  useDomainLimits,
  useResetDomainLimits,
  useSetDomainLimits,
  type DomainLimits,
} from '../../queries/domainLimits'

const MB = 1024 * 1024

function toMB(bytes: number): string {
  return bytes > 0 ? String(Math.round((bytes / MB) * 100) / 100) : ''
}

function toNum(v: string): number {
  return v.trim() === '' ? 0 : Number(v)
}

type Problem = 'body' | 'read' | 'write' | null

function validate(
  l: DomainLimits,
  body: string,
  read: string,
  write: string,
): Problem {
  const b = toNum(body)
  const bytes = Math.round(b * MB)
  if (!Number.isFinite(b) || b < 0) return 'body'
  if (bytes !== 0 && (bytes < l.min_body_bytes || bytes > l.ceiling_body_bytes))
    return 'body'
  for (const [key, v] of [
    ['read', toNum(read)],
    ['write', toNum(write)],
  ] as const) {
    if (!Number.isInteger(v) || v < 0 || v > l.ceiling_timeout_seconds)
      return key
  }
  return null
}

function Row({
  limits,
  appName,
  domain,
}: {
  limits: DomainLimits
  appName: string
  domain: string
}) {
  const { t } = useTranslation('domains')
  const set = useSetDomainLimits(appName, domain)
  const reset = useResetDomainLimits(appName, domain)
  const [body, setBody] = useState(toMB(limits.max_body_bytes))
  const [read, setRead] = useState(
    limits.read_timeout_seconds > 0 ? String(limits.read_timeout_seconds) : '',
  )
  const [write, setWrite] = useState(
    limits.write_timeout_seconds > 0
      ? String(limits.write_timeout_seconds)
      : '',
  )

  const problem = validate(limits, body, read, write)
  const bytes = Math.round(toNum(body) * MB)
  const unchanged =
    bytes === limits.max_body_bytes &&
    toNum(read) === limits.read_timeout_seconds &&
    toNum(write) === limits.write_timeout_seconds
  const ceilingMB = Math.floor(limits.ceiling_body_bytes / MB)
  const minMB = Math.ceil(limits.min_body_bytes / MB)
  const error = set.error ?? reset.error

  const submit = () => {
    set.mutate(
      {
        max_body_bytes: bytes,
        read_timeout_seconds: toNum(read),
        write_timeout_seconds: toNum(write),
      },
      {
        onSuccess: () =>
          toast.add({ title: t('limits.saved'), type: 'success' }),
      },
    )
  }

  const fields = [
    {
      id: 'body',
      label: t('limits.bodyLabel'),
      value: body,
      set: setBody,
      hint: t('limits.bodyHint', { min: minMB, max: ceilingMB }),
      step: 'any',
    },
    {
      id: 'read',
      label: t('limits.readLabel'),
      value: read,
      set: setRead,
      hint: t('limits.timeoutHint', { max: limits.ceiling_timeout_seconds }),
      step: '1',
    },
    {
      id: 'write',
      label: t('limits.writeLabel'),
      value: write,
      set: setWrite,
      hint: t('limits.timeoutHint', { max: limits.ceiling_timeout_seconds }),
      step: '1',
    },
  ] as const

  return (
    <form
      className="space-y-2 rounded-md border border-border bg-muted/30 p-3 text-sm"
      onSubmit={(e) => {
        e.preventDefault()
        if (!problem && !unchanged) submit()
      }}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="font-medium">{t('limits.title')}</span>
        <Badge variant={limits.configured ? 'default' : 'muted'}>
          {limits.configured ? t('limits.configured') : t('limits.default')}
        </Badge>
      </div>
      <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
        {fields.map((f) => (
          <div key={f.id} className="min-w-0 space-y-1">
            <label
              htmlFor={`limits-${f.id}`}
              className="text-xs font-medium text-muted-foreground"
            >
              {f.label}
            </label>
            <Input
              id={`limits-${f.id}`}
              type="number"
              inputMode="decimal"
              min={0}
              step={f.step}
              placeholder="0"
              value={f.value}
              aria-invalid={problem === f.id}
              aria-describedby={`limits-${f.id}-hint`}
              onChange={(e) => {
                f.set(e.target.value)
              }}
            />
            <p
              id={`limits-${f.id}-hint`}
              className={
                problem === f.id
                  ? 'text-xs text-destructive'
                  : 'text-xs text-muted-foreground'
              }
            >
              {f.hint}
            </p>
          </div>
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="submit"
          size="sm"
          disabled={problem !== null || unchanged || set.isPending}
        >
          {t('limits.save')}
        </Button>
        {limits.configured ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={reset.isPending}
            onClick={() => {
              reset.mutate(undefined, {
                onSuccess: () =>
                  toast.add({ title: t('limits.resetDone'), type: 'success' }),
              })
            }}
          >
            {t('limits.reset')}
          </Button>
        ) : null}
        <span className="text-xs text-muted-foreground">
          {t('limits.note')}
        </span>
      </div>
      {error ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{error.message}</AlertDescription>
        </Alert>
      ) : null}
    </form>
  )
}

export function DomainTrafficLimitsControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { data, isLoading } = useDomainLimits(appName, domain)
  if (isLoading || !data) {
    return isLoading ? (
      <Skeleton className="h-24 w-full" aria-hidden="true" />
    ) : null
  }
  // Keyed on the saved values so a save or reset re-prefills the draft.
  const key = `${data.max_body_bytes}-${data.read_timeout_seconds}-${data.write_timeout_seconds}`
  return <Row key={key} limits={data} appName={appName} domain={domain} />
}
