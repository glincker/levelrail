import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  useFailureSimulation,
  usePlacementSimulation,
} from '../../queries/placementDecision'
import type {
  SimulatedPlacement,
  ZoneSummary,
} from '../../types/placementDecision'

const MAX_ROWS = 50

function Rows({ rows }: { rows: SimulatedPlacement[] }) {
  const { t } = useTranslation('nodes')
  return (
    <ul className="divide-y divide-border rounded-lg border border-border text-xs">
      {rows.slice(0, MAX_ROWS).map((p) => (
        <li key={p.name} className="flex flex-wrap gap-x-3 gap-y-1 px-3 py-2">
          <span className="font-medium text-foreground">{p.name}</span>
          {p.from_node_id ? (
            <span className="text-muted-foreground">
              {t('zones.whatIf.from')}: {p.from_node_id}
            </span>
          ) : null}
          <span className={p.unplaced ? 'text-destructive' : 'text-foreground'}>
            {t('zones.whatIf.to')}:{' '}
            {p.unplaced
              ? t('zones.whatIf.none')
              : `${p.node_name || p.node_id}${p.zone ? ` (${p.zone})` : ''}`}
          </span>
          <span className="text-muted-foreground">{p.unplaced || p.why}</span>
        </li>
      ))}
    </ul>
  )
}

function ZoneFailure({ zones }: { zones: ZoneSummary[] }) {
  const { t } = useTranslation('nodes')
  const [zone, setZone] = useState<string | null>(null)
  const sim = useFailureSimulation(zone)
  const named = zones.filter((z) => z.zone !== '')
  return (
    <div className="space-y-2">
      <p className="text-xs text-muted-foreground">
        {t('zones.whatIf.failureHint')}
      </p>
      <div className="flex flex-wrap gap-2">
        {named.map((z) => (
          <Button
            key={z.zone}
            type="button"
            size="sm"
            variant={zone === z.zone ? 'default' : 'outline'}
            onClick={() => setZone(z.zone)}
          >
            {t('zones.whatIf.failZone', { zone: z.zone })}
          </Button>
        ))}
      </div>
      {sim.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('zones.whatIf.failed')} {sim.error.message}
          </AlertDescription>
        </Alert>
      ) : null}
      {sim.data ? (
        <div className="space-y-2">
          <p className="text-sm text-foreground">
            {t('zones.whatIf.failureSummary', {
              lost: sim.data.lost_node_ids.length,
              displaced: sim.data.displaced,
              stuck: sim.data.unplaced,
            })}
          </p>
          <p
            className={
              sim.data.unplaced > 0
                ? 'text-xs text-destructive'
                : 'text-xs text-muted-foreground'
            }
          >
            {sim.data.unplaced > 0
              ? t('zones.whatIf.failureStuck')
              : t('zones.whatIf.failureOk')}
          </p>
          {sim.data.placed.length > 0 ? <Rows rows={sim.data.placed} /> : null}
        </div>
      ) : null}
    </div>
  )
}

function AddApps() {
  const { t } = useTranslation('nodes')
  const [memory, setMemory] = useState('512')
  const [cpu, setCpu] = useState('0')
  const [count, setCount] = useState('1')
  const [query, setQuery] = useState<string | null>(null)
  const [invalid, setInvalid] = useState(false)
  const sim = usePlacementSimulation(query)

  const run = () => {
    const apps = Number(count)
    if (
      [memory, cpu, count].some((v) => {
        const n = Number(v)
        return !Number.isInteger(n) || n < 0
      }) ||
      apps < 1 ||
      apps > 200
    ) {
      setInvalid(true)
      return
    }
    setInvalid(false)
    setQuery(
      new URLSearchParams({
        memory_mib: memory,
        cpu_milli: cpu,
        count,
      }).toString(),
    )
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <Label htmlFor="sim-memory">{t('zones.whatIf.addMemory')}</Label>
          <Input
            id="sim-memory"
            inputMode="numeric"
            value={memory}
            onChange={(e) => setMemory(e.target.value)}
            className="w-32"
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="sim-cpu">{t('zones.whatIf.addCpu')}</Label>
          <Input
            id="sim-cpu"
            inputMode="numeric"
            value={cpu}
            onChange={(e) => setCpu(e.target.value)}
            className="w-32"
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor="sim-count">{t('zones.whatIf.addCount')}</Label>
          <Input
            id="sim-count"
            inputMode="numeric"
            value={count}
            onChange={(e) => setCount(e.target.value)}
            className="w-24"
          />
        </div>
        <Button type="button" size="sm" onClick={run}>
          {t('zones.whatIf.run')}
        </Button>
      </div>
      {invalid ? (
        <p className="text-xs text-destructive">{t('zones.whatIf.invalid')}</p>
      ) : null}
      {sim.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('zones.whatIf.failed')} {sim.error.message}
          </AlertDescription>
        </Alert>
      ) : null}
      {sim.data ? <Rows rows={sim.data.placed} /> : null}
    </div>
  )
}

export function PlacementWhatIf({ zones }: { zones: ZoneSummary[] }) {
  const { t } = useTranslation('nodes')
  return (
    <div className="space-y-4 border-t border-border pt-4">
      <p className="text-sm font-medium text-foreground">
        {t('zones.whatIf.title')}
      </p>
      <ZoneFailure zones={zones} />
      <p className="text-sm font-medium text-foreground">
        {t('zones.whatIf.addTitle')}
      </p>
      <AddApps />
    </div>
  )
}
