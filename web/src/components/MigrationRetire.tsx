import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  ClockGlassIcon,
  HandGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useRetireReport,
  useRunRetire,
  type GateStatus,
  type RetireGate,
} from '../queries/migrationJob'
import type { AppImportSession } from '../queries/appImport'

const MANUAL_GATES = ['source-backup', 'traffic-cycle', 'leftovers'] as const

const statusIcon: Record<GateStatus, typeof CircleCheckGlassIcon> = {
  pass: CircleCheckGlassIcon,
  fail: CircleXGlassIcon,
  pending: ClockGlassIcon,
  manual: HandGlassIcon,
}

const statusTone: Record<GateStatus, string> = {
  pass: 'text-emerald-600 dark:text-emerald-400',
  fail: 'text-destructive',
  pending: 'text-muted-foreground',
  manual: 'text-amber-600 dark:text-amber-400',
}

function storageKey(id: string): string {
  return `migration-acks-${id}`
}

function loadAcks(id: string): string[] {
  try {
    const raw = window.localStorage.getItem(storageKey(id))
    const parsed: unknown = raw ? JSON.parse(raw) : []
    return Array.isArray(parsed)
      ? parsed.filter((x): x is string => typeof x === 'string')
      : []
  } catch {
    return []
  }
}

function saveAcks(id: string, acks: string[]) {
  try {
    window.localStorage.setItem(storageKey(id), JSON.stringify(acks))
  } catch {
    // Storage can be blocked; the confirmations then last for this visit only.
  }
}

function Gate({
  gate,
  acknowledged,
  onConfirm,
}: {
  gate: RetireGate
  acknowledged: boolean
  onConfirm: () => void
}) {
  const { t } = useTranslation('migration')
  const Icon = statusIcon[gate.status]
  const canConfirm =
    gate.status === 'manual' &&
    !acknowledged &&
    (MANUAL_GATES as readonly string[]).includes(gate.id)
  return (
    <li className="space-y-1 rounded-md border p-3 text-sm">
      <div className="flex flex-wrap items-center gap-2">
        <Icon
          className={`size-4 shrink-0 ${statusTone[gate.status]}`}
          aria-hidden
        />
        <span className="font-medium">
          {t(`appImport.retire.gate.${gate.id}`, { defaultValue: gate.id })}
        </span>
        <span className="text-xs text-muted-foreground">
          {t(`appImport.retire.status.${gate.status}`)}
        </span>
        {canConfirm ? (
          <Button
            size="sm"
            variant="outline"
            className="ml-auto"
            onClick={onConfirm}
          >
            {t('appImport.retire.confirm')}
          </Button>
        ) : null}
      </div>
      <p className="text-muted-foreground">{gate.detail}</p>
      {gate.open && gate.open.length > 0 ? (
        <ul className="list-disc pl-5 text-xs break-words">
          {gate.open.map((o) => (
            <li key={o}>{o}</li>
          ))}
        </ul>
      ) : null}
      {gate.status !== 'pass' && gate.fix ? (
        <p className="text-xs">
          {t('appImport.retire.next')} {gate.fix}
        </p>
      ) : null}
    </li>
  )
}

export default function MigrationRetire({
  session,
  onBack,
}: {
  session: AppImportSession
  onBack: () => void
}) {
  const { t } = useTranslation('migration')
  const [acks, setAcks] = useState<string[]>(() => loadAcks(session.id))
  const report = useRetireReport(session.id, acks)
  const run = useRunRetire()
  const [targetIp, setTargetIp] = useState('')
  const data = report.data

  function check(probe: boolean, next: string[]) {
    run.mutate({
      id: session.id,
      probe,
      acknowledged: next,
      targetIp: targetIp.trim() || undefined,
    })
  }

  function confirm(id: string) {
    const next = acks.includes(id) ? acks : [...acks, id]
    setAcks(next)
    saveAcks(session.id, next)
    check(false, next)
  }

  const open = data ? data.gates.filter((g) => g.status !== 'pass').length : 0
  const error = report.error ?? run.error

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {t('appImport.retire.title')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('appImport.retire.description')}
        </p>
      </div>
      <div className="flex flex-wrap items-end gap-2">
        <div className="min-w-48 space-y-1.5">
          <Label htmlFor="appimport-retire-ip">
            {t('appImport.retire.targetIp')}
          </Label>
          <Input
            id="appimport-retire-ip"
            placeholder="203.0.113.10"
            value={targetIp}
            onChange={(e) => setTargetIp(e.target.value)}
          />
        </div>
        <Button disabled={run.isPending} onClick={() => check(true, acks)}>
          {run.isPending
            ? t('appImport.retire.running')
            : t('appImport.retire.run')}
        </Button>
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription className="break-words">
            {error.message}
          </AlertDescription>
        </Alert>
      ) : null}
      {data ? (
        <>
          <div aria-live="polite">
            {data.safe_to_delete ? (
              <Banner variant="success" title={t('appImport.retire.safeTitle')}>
                {t('appImport.retire.safeBody')}
              </Banner>
            ) : (
              <Banner
                variant="warning"
                title={t('appImport.retire.notSafeTitle')}
              >
                {t('appImport.retire.notSafeBody', { count: open })}
              </Banner>
            )}
          </div>
          <ul className="space-y-2">
            {data.gates.map((g) => (
              <Gate
                key={g.id}
                gate={g}
                acknowledged={acks.includes(g.id)}
                onConfirm={() => confirm(g.id)}
              />
            ))}
          </ul>
          <div className="space-y-1 rounded-md border bg-muted/30 p-3 text-sm">
            <p className="font-medium">{t('appImport.retire.fallbackTitle')}</p>
            <ul className="list-disc space-y-1 pl-5 text-muted-foreground">
              {data.fallback.map((f) => (
                <li key={f}>{f}</li>
              ))}
            </ul>
          </div>
        </>
      ) : null}
      <Button variant="outline" onClick={onBack}>
        {t('appImport.retire.back')}
      </Button>
    </div>
  )
}
