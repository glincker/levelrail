import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { rollingDrainOrder } from '../../lib/rollingDrain'
import { useZones } from '../../queries/placementDecision'
import { fetchNodeDrain, startNodeDrain } from '../../queries/nodeMoves'
import type { NodeDrain } from '../../types/nodeMoves'

const POLL_MS = 2_000

type StepState = 'pending' | 'running' | 'succeeded' | 'partial' | 'failed'

const VARIANT: Record<
  StepState,
  'muted' | 'success' | 'warning' | 'destructive'
> = {
  pending: 'muted',
  running: 'muted',
  succeeded: 'success',
  partial: 'warning',
  failed: 'destructive',
}

interface Step {
  nodeId: string
  state: StepState
  moved: number
  total: number
  error?: string
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// Zone drain is orchestrated here like the CLI: one node at a time, waiting
// for each drain to finish and stopping at the first one that does not succeed.
export function ZoneDrainPanel({
  initialZone,
  onClose,
  onBack,
}: {
  initialZone: string
  onClose: () => void
  onBack?: () => void
}) {
  const { t } = useTranslation('nodes')
  const zones = useZones()
  const [zone, setZone] = useState(initialZone)
  const [minAvailable, setMinAvailable] = useState('1')
  const [steps, setSteps] = useState<Step[] | null>(null)
  const [stopped, setStopped] = useState<string | null>(null)
  const [preview, setPreview] = useState<string[] | null>(null)
  const [problem, setProblem] = useState<string | null>(null)
  const [running, setRunning] = useState(false)
  const cancelled = useRef(false)

  useEffect(() => {
    cancelled.current = false
    return () => {
      cancelled.current = true
    }
  }, [])

  const nodes = zones.data?.nodes ?? []
  const names = new Map(nodes.map((n) => [n.node_id, n.name]))
  const zoneNames = (zones.data?.zones ?? [])
    .map((z) => z.zone)
    .filter((z) => z !== '')

  function plan(): string[] | null {
    const min = Number(minAvailable)
    if (!Number.isInteger(min) || min < 1) {
      setProblem(t('drain.zone.invalidMin'))
      return null
    }
    const toRolling = (n: (typeof nodes)[number]) => ({
      id: n.node_id,
      online: n.online,
      schedulable: true,
      workloads: n.workloads,
    })
    const result = rollingDrainOrder(
      nodes.filter((n) => n.zone === zone).map(toRolling),
      nodes.filter((n) => n.zone !== zone).map(toRolling),
      min,
    )
    if (!result.ok) {
      setProblem(
        result.reason === 'empty'
          ? t('drain.zone.empty', { zone })
          : t('drain.zone.tooFew', { stay: result.stay, min }),
      )
      return null
    }
    setProblem(null)
    return result.order
  }

  function handlePreview() {
    setPreview(plan())
  }

  function patch(index: number, next: Partial<Step>) {
    setSteps((cur) =>
      cur ? cur.map((s, i) => (i === index ? { ...s, ...next } : s)) : cur,
    )
  }

  async function handleStart() {
    const order = plan()
    if (!order) {
      return
    }
    setPreview(null)
    setStopped(null)
    setRunning(true)
    setSteps(
      order.map((nodeId) => ({
        nodeId,
        state: 'pending',
        moved: 0,
        total: 0,
      })),
    )
    for (const [i, nodeId] of order.entries()) {
      patch(i, { state: 'running' })
      try {
        let d: NodeDrain = await startNodeDrain(nodeId, '')
        for (;;) {
          patch(i, {
            moved: d.progress?.succeeded ?? 0,
            total: d.progress?.total ?? 0,
          })
          if (d.status !== 'running') {
            break
          }
          await wait(POLL_MS)
          if (cancelled.current) {
            return
          }
          d = await fetchNodeDrain(d.id)
        }
        patch(i, { state: d.status, error: d.error })
        if (d.status !== 'succeeded') {
          setStopped(t('drain.zone.stopped', { left: order.length - i - 1 }))
          break
        }
      } catch (err) {
        patch(i, {
          state: 'failed',
          error: err instanceof Error ? err.message : String(err),
        })
        setStopped(t('drain.zone.stopped', { left: order.length - i - 1 }))
        break
      }
    }
    if (!cancelled.current) {
      setRunning(false)
    }
  }

  const label = (id: string) => names.get(id) ?? id

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <Label htmlFor="drain-zone">{t('drain.zone.zoneLabel')}</Label>
          <NativeSelect
            id="drain-zone"
            value={zone}
            disabled={running}
            onChange={(e) => {
              setZone(e.target.value)
              setPreview(null)
            }}
          >
            <option value="" disabled>
              {t('drain.zone.pick')}
            </option>
            {zoneNames.map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </NativeSelect>
        </div>
        <div className="space-y-1">
          <Label htmlFor="drain-min-available">
            {t('drain.zone.minAvailable')}
          </Label>
          <Input
            id="drain-min-available"
            inputMode="numeric"
            className="w-20"
            value={minAvailable}
            disabled={running}
            onChange={(e) => setMinAvailable(e.target.value)}
          />
        </div>
        <Button
          type="button"
          variant="outline"
          disabled={running || zone === ''}
          onClick={handlePreview}
        >
          {t('drain.zone.dryRun')}
        </Button>
        <Button
          type="button"
          disabled={running || zone === ''}
          onClick={() => void handleStart()}
        >
          {running ? t('drain.starting') : t('drain.zone.start')}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">{t('drain.zone.hint')}</p>
      {zones.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{zones.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {problem ? (
        <Alert variant="destructive">
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      ) : null}
      {preview ? (
        <div className="space-y-1">
          <p className="text-xs text-muted-foreground">
            {t('drain.zone.dryRunResult', { count: preview.length, zone })}
          </p>
          <ol className="list-decimal space-y-0.5 pl-5 text-sm">
            {preview.map((id) => (
              <li key={id}>{label(id)}</li>
            ))}
          </ol>
        </div>
      ) : null}
      {steps ? (
        <ul
          className="divide-y divide-border rounded-lg border border-border text-sm"
          aria-live="polite"
        >
          {steps.map((s) => (
            <li
              key={s.nodeId}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2"
            >
              <span className="font-medium text-foreground">
                {label(s.nodeId)}
              </span>
              <Badge variant={VARIANT[s.state]}>
                {t(`drain.zone.step.${s.state}`)}
              </Badge>
              {s.total > 0 ? (
                <span className="text-xs text-muted-foreground">
                  {t('drain.zone.moved', { done: s.moved, total: s.total })}
                </span>
              ) : null}
              {s.error ? (
                <span className="text-xs text-destructive">{s.error}</span>
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
      {stopped ? <p className="text-xs text-destructive">{stopped}</p> : null}
      <div className="flex justify-end gap-2">
        {onBack ? (
          <Button
            type="button"
            variant="ghost"
            disabled={running}
            onClick={onBack}
          >
            {t('drain.zone.back')}
          </Button>
        ) : null}
        <Button type="button" variant="outline" onClick={onClose}>
          {t('drain.close')}
        </Button>
      </div>
    </div>
  )
}
