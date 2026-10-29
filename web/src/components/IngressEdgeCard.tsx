import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { GaugeGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import {
  edgeSettingsQueryOptions,
  useEdgeSettingsActions,
  type EdgeSettings,
  type EdgeSettingsUpdate,
} from '../queries/ingressEdge'
import { TrustedProxyFix } from './TrustedProxyFix'

const KIB = 1024
const MIB = 1024 * 1024

interface Draft {
  trusted: string
  body: string
  header: string
  readHeader: string
  idle: string
}

function toDraft(s: EdgeSettings): Draft {
  const show = (v: number | null, unit: number) =>
    v === null ? '' : String(v / unit)
  return {
    trusted: s.trusted_proxies,
    body: show(s.max_body_bytes, MIB),
    header: show(s.max_header_bytes, KIB),
    readHeader: show(s.read_header_timeout_secs, 1),
    idle: show(s.idle_timeout_secs, 1),
  }
}

// Blank means the built-in default; null means the entry is not a number.
function parse(raw: string, unit: number): number | null | undefined {
  if (raw.trim() === '') return null
  const n = Number(raw)
  if (!Number.isFinite(n) || n < 0) return undefined
  return Math.round(n * unit)
}

function build(d: Draft): EdgeSettingsUpdate | null {
  const body = parse(d.body, MIB)
  const header = parse(d.header, KIB)
  const readHeader = parse(d.readHeader, 1)
  const idle = parse(d.idle, 1)
  if ([body, header, readHeader, idle].includes(undefined)) return null
  return {
    trusted_proxies: d.trusted.trim(),
    max_body_bytes: body ?? null,
    max_header_bytes: header ?? null,
    read_header_timeout_secs: readHeader ?? null,
    idle_timeout_secs: idle ?? null,
  }
}

function EdgeForm({ settings }: { settings: EdgeSettings }) {
  const { t } = useTranslation('domains')
  const isRoot = useIsRoot()
  const { save } = useEdgeSettingsActions()
  const [draft, setDraft] = useState<Draft>(() => toDraft(settings))
  const update = build(draft)
  const set = (key: keyof Draft) => (value: string) => {
    setDraft((d) => ({ ...d, [key]: value }))
  }
  const eff = settings.effective
  const rows: {
    key: keyof Draft
    locked: string
    label:
      | 'edge.limits.body'
      | 'edge.limits.header'
      | 'edge.limits.readHeader'
      | 'edge.limits.idle'
    unit: 'edge.unit.mib' | 'edge.unit.kib' | 'edge.unit.seconds'
    current: string
  }[] = [
    {
      key: 'body',
      locked: 'max_body_bytes',
      label: 'edge.limits.body',
      unit: 'edge.unit.mib',
      current: String(Math.round(eff.max_body_bytes.value / MIB)),
    },
    {
      key: 'header',
      locked: 'max_header_bytes',
      label: 'edge.limits.header',
      unit: 'edge.unit.kib',
      current: String(Math.round(eff.max_header_bytes.value / KIB)),
    },
    {
      key: 'readHeader',
      locked: 'read_header_timeout_secs',
      label: 'edge.limits.readHeader',
      unit: 'edge.unit.seconds',
      current: String(eff.read_header_timeout_secs.value),
    },
    {
      key: 'idle',
      locked: 'idle_timeout_secs',
      label: 'edge.limits.idle',
      unit: 'edge.unit.seconds',
      current: String(eff.idle_timeout_secs.value),
    },
  ]

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!update) return
        save.mutate(update, {
          onSuccess: () => {
            toast.add({ title: t('edge.saved'), type: 'success' })
          },
          onError: (err) => {
            toast.add({ title: err.message, type: 'error' })
          },
        })
      }}
    >
      <Field>
        <FieldLabel htmlFor="edge-trusted">
          {t('edge.trusted.label')}
        </FieldLabel>
        <Input
          id="edge-trusted"
          className="font-mono"
          placeholder="private_ranges, 203.0.113.0/24"
          disabled={!isRoot || settings.env_locked.trusted_proxies}
          value={draft.trusted}
          onChange={(e) => {
            set('trusted')(e.target.value)
          }}
        />
        <FieldDescription>
          {settings.env_locked.trusted_proxies
            ? t('edge.envLocked')
            : t('edge.trusted.help')}
        </FieldDescription>
      </Field>
      <div className="grid gap-4 sm:grid-cols-2">
        {rows.map((r) => (
          <Field key={r.key}>
            <FieldLabel htmlFor={`edge-${r.key}`}>{t(r.label)}</FieldLabel>
            <div className="flex items-center gap-2">
              <Input
                id={`edge-${r.key}`}
                inputMode="decimal"
                className="w-28 font-mono"
                placeholder={r.current}
                disabled={!isRoot || settings.env_locked[r.locked]}
                value={draft[r.key]}
                onChange={(e) => {
                  set(r.key)(e.target.value)
                }}
              />
              <span className="text-sm text-muted-foreground">{t(r.unit)}</span>
            </div>
            <FieldDescription>
              {settings.env_locked[r.locked]
                ? t('edge.envLocked')
                : t('edge.limits.help')}
            </FieldDescription>
          </Field>
        ))}
      </div>
      {!update ? (
        <Alert variant="destructive">
          <AlertDescription>{t('edge.badNumber')}</AlertDescription>
        </Alert>
      ) : null}
      {isRoot ? (
        <Button type="submit" size="sm" disabled={!update || save.isPending}>
          {save.isPending ? t('edge.saving') : t('edge.save')}
        </Button>
      ) : null}
    </form>
  )
}

export function IngressEdgeCard() {
  const { t } = useTranslation('domains')
  const { data, isPending, isError } = useQuery(edgeSettingsQueryOptions())

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GaugeGlassIcon className="size-5" aria-hidden="true" />
          {t('edge.title')}
        </CardTitle>
        <CardDescription>{t('edge.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <TrustedProxyFix />
        {isPending ? <Skeleton className="h-32 w-full" /> : null}
        {isError ? (
          <Alert variant="destructive">
            <AlertDescription>{t('edge.loadFailed')}</AlertDescription>
          </Alert>
        ) : null}
        {data ? <EdgeForm key={JSON.stringify(data)} settings={data} /> : null}
      </CardContent>
    </Card>
  )
}
