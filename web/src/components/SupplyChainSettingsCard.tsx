import { useState } from 'react'
import { ShieldCheckIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { InfoTip, SkeletonLine } from './kit'
import {
  useOverrideScanGate,
  useSetSupplyChain,
  useSupplyChainSettings,
} from '../queries/supplyChain'
import type { ScanGate, SupplyChainSettings } from '../types/supplyChain'

interface GateOption {
  gate: ScanGate
  label: string
  summary: string
  detail: string
}

const GATES: GateOption[] = [
  {
    gate: 'off',
    label: 'Report only',
    summary: 'Scan and record, never hold a release.',
    detail:
      'Every build is scanned and the counts are shown per deploy, but a release always goes live.',
  },
  {
    gate: 'warn',
    label: 'Warn',
    summary: 'Release anyway, flag critical findings.',
    detail:
      'A release with critical vulnerabilities still goes live. The deploy is marked with a warning and the reason.',
  },
  {
    gate: 'block_on_critical',
    label: 'Block on critical',
    summary: 'Keep the previous release serving.',
    detail:
      'A release with a critical vulnerability does not go live. The previous release keeps serving and the deploy records why. If the scanner itself fails, the release is allowed.',
  },
]

function GateSelector({
  value,
  disabled,
  onChange,
}: {
  value: ScanGate
  disabled: boolean
  onChange: (gate: ScanGate) => void
}) {
  return (
    <div role="radiogroup" aria-label="Scan gate" className="grid gap-2">
      {GATES.map((g) => {
        const id = `scan-gate-${g.gate}`
        const selected = value === g.gate
        return (
          <div
            key={g.gate}
            className={cn(
              'flex items-start gap-2 rounded-md border p-3',
              selected ? 'border-primary bg-muted/50' : 'border-border',
              disabled && 'opacity-60',
            )}
          >
            <label
              htmlFor={id}
              className={cn(
                'grid min-w-0 flex-1 cursor-pointer grid-cols-[auto_1fr] items-start gap-x-3 text-sm font-medium text-foreground',
                disabled && 'cursor-not-allowed',
              )}
            >
              <input
                id={id}
                type="radio"
                name="scan-gate"
                className="mt-1 row-span-2 accent-primary"
                checked={selected}
                disabled={disabled}
                onChange={() => onChange(g.gate)}
              />
              {g.label}
              <span className="font-normal text-muted-foreground">
                {g.summary}
              </span>
            </label>
            <InfoTip label={`About ${g.label}`}>{g.detail}</InfoTip>
          </div>
        )
      })}
    </div>
  )
}

// SupplyChainSettingsCard turns vulnerability scanning on per app and picks
// what a scan may do to a release (GET/PUT /api/v1/apps/{name}/supply-chain).
export function SupplyChainSettingsCard({ appName }: { appName: string }) {
  const settings = useSupplyChainSettings(appName)
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckIcon className="size-4 text-muted-foreground" />
          Supply chain
          <InfoTip label="About vulnerability scanning">
            Each Dockerfile build can record a software bill of materials. When
            scanning is on, a scanner container checks it for known
            vulnerabilities before the release goes live.
          </InfoTip>
        </CardTitle>
        <CardDescription>
          Scan the software bill of materials of each deploy for known
          vulnerabilities, and optionally hold releases with critical ones.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {settings.isPending ? (
          <div className="space-y-2" aria-label="Loading supply chain settings">
            <SkeletonLine width="60%" />
            <SkeletonLine width="40%" />
          </div>
        ) : settings.isError ? (
          <p className="text-sm text-muted-foreground">
            Supply chain visibility is not available on this control plane.
          </p>
        ) : (
          <SupplyChainForm appName={appName} settings={settings.data} />
        )}
      </CardContent>
    </Card>
  )
}

function SupplyChainForm({
  appName,
  settings,
}: {
  appName: string
  settings: SupplyChainSettings
}) {
  const save = useSetSupplyChain(appName)
  const override = useOverrideScanGate(appName)
  const [reason, setReason] = useState('')
  const blocked = settings.scan_gate === 'block_on_critical'

  function onError(title: string) {
    return (error: Error) =>
      toast.add({ title, description: error.message, type: 'error' })
  }

  function toggle() {
    const next = !settings.scan_enabled
    save.mutate(
      next ? { scan_enabled: true } : { scan_enabled: false, scan_gate: 'off' },
      {
        onSuccess: () =>
          toast.add({
            title: next ? 'Vulnerability scanning on.' : 'Scanning turned off.',
            type: 'success',
          }),
        onError: onError('Could not update scanning.'),
      },
    )
  }

  function changeGate(gate: ScanGate) {
    save.mutate(
      { scan_gate: gate },
      {
        onSuccess: () =>
          toast.add({ title: 'Scan gate updated.', type: 'success' }),
        onError: onError('Could not update the scan gate.'),
      },
    )
  }

  function arm() {
    override.mutate(reason.trim(), {
      onSuccess: () => {
        setReason('')
        toast.add({
          title: 'Override armed for the next release.',
          type: 'success',
        })
      },
      onError: onError('Could not arm the override.'),
    })
  }

  return (
    <div className="space-y-5">
      {!settings.server_enabled && (
        <p className="rounded-md border border-border bg-muted/50 p-3 text-sm text-muted-foreground">
          Scanning is switched off on this server. Set{' '}
          <code className="font-mono text-xs">APP_SCAN_ENABLED=true</code> and
          restart the control plane to allow it.
        </p>
      )}
      {!settings.build_attest && (
        <p className="rounded-md border border-border bg-muted/50 p-3 text-sm text-muted-foreground">
          Builds do not record a software bill of materials yet, so there is
          nothing to scan. Set{' '}
          <code className="font-mono text-xs">APP_BUILD_ATTEST=true</code> and
          restart the control plane. It applies to Dockerfile builds only.
        </p>
      )}

      <div className="flex items-start justify-between gap-3 rounded-md border border-border p-3">
        <div className="min-w-0 space-y-0.5">
          <p className="text-sm font-medium">
            Scan each deploy
            <span className="ml-1.5 align-middle">
              <InfoTip label="About the scanner cost">
                Off by default. The first scan pulls the {settings.scanner}{' '}
                scanner image ({settings.scanner_image}, roughly 100 to 250 MB,
                size not measured) and its vulnerability database, then reuses
                both. Each scan runs a short-lived container capped at 1 GB of
                memory, and a deploy waits for it before going live.
              </InfoTip>
            </span>
          </p>
          <p className="text-sm text-muted-foreground">
            Uses {settings.scanner}, downloaded on the first scan.
          </p>
        </div>
        <Button
          size="sm"
          variant={settings.scan_enabled ? 'outline' : 'default'}
          onClick={toggle}
          disabled={save.isPending || !settings.server_enabled}
          aria-pressed={settings.scan_enabled}
        >
          {settings.scan_enabled ? 'Turn off' : 'Turn on'}
        </Button>
      </div>

      {settings.scan_enabled && (
        <>
          <GateSelector
            value={settings.scan_gate}
            disabled={save.isPending}
            onChange={changeGate}
          />
          {blocked && (
            <div className="space-y-1.5">
              <Label
                htmlFor="scan-override"
                className="flex items-center gap-1.5"
              >
                Let the next blocked release through
                <InfoTip label="About the override">
                  Applies to one release and expires after an hour. Your reason
                  is recorded in the app timeline.
                </InfoTip>
              </Label>
              {settings.override_armed ? (
                <p className="text-sm text-muted-foreground">
                  Armed: {settings.override_reason}
                </p>
              ) : (
                <div className="flex flex-wrap gap-2">
                  <Input
                    id="scan-override"
                    className="min-w-0 flex-1"
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    placeholder="Why this release may go live"
                  />
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={arm}
                    disabled={reason.trim() === '' || override.isPending}
                  >
                    Arm override
                  </Button>
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  )
}
