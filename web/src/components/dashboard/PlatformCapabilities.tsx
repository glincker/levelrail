import { useState } from 'react'
import {
  ArrowsClockwiseIcon,
  DatabaseIcon,
  ListChecksIcon,
  PulseIcon,
  RobotIcon,
  ShareNetworkIcon,
  ShieldCheckIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { useBrand } from '../../hooks/useBrand'

interface Capability {
  key: string
  icon: Icon
  label: string
  detail: string
}

const CAPABILITIES: Capability[] = [
  {
    key: 'deploys',
    icon: ArrowsClockwiseIcon,
    label: 'Zero-downtime deploys',
    detail:
      'Rolling, recreate, or blue-green strategy, gated on real readiness and liveness probes. Rollback to a pinned prior image is always available.',
  },
  {
    key: 'observability',
    icon: PulseIcon,
    label: 'Observability built in',
    detail:
      'Node-local metrics at 15s resolution and full-text log search, no separate Grafana or Loki install. Deploy markers overlay directly on the metric charts.',
  },
  {
    key: 'databases',
    icon: DatabaseIcon,
    label: 'Eight managed databases',
    detail:
      'Postgres, Redis, MySQL, MongoDB, MariaDB, KeyDB, Dragonfly, and ClickHouse, with scheduled backups, restore, and automatic post-backup verification.',
  },
  {
    key: 'mesh',
    icon: ShareNetworkIcon,
    label: 'Multi-node from day one',
    detail:
      'A WireGuard mesh and internal DNS across nodes, with cordon and drain. No inbound ports are required on any managed server.',
  },
  {
    key: 'attention',
    icon: ListChecksIcon,
    label: 'Know what needs attention',
    detail:
      'The Status page and the attention CLI command list failing apps, offline nodes, and expiring certificates, with a disk pressure banner and stalled renewal detection.',
  },
  {
    key: 'iam',
    icon: ShieldCheckIcon,
    label: 'Resource-scoped IAM',
    detail:
      'AWS-IAM-shaped Allow/Deny policies scoped to a specific app or database, with a full audit log and CSV export, in the free Apache 2.0 core.',
  },
  {
    key: 'mcp',
    icon: RobotIcon,
    label: 'AI-ready API',
    detail:
      '144 MCP tools (beta), backed by the same HTTP API the dashboard runs on, so AI tools can list apps, read logs, and diagnose a crashloop directly.',
  },
]

// Click-to-select, not a feature grid: one capability expands at a time so
// a brand-new operator skims labels first and reads detail only on demand.
export function PlatformCapabilities() {
  const brand = useBrand()
  const [selectedKey, setSelectedKey] = useState(CAPABILITIES[0]!.key)
  const selected =
    CAPABILITIES.find((capability) => capability.key === selectedKey) ??
    CAPABILITIES[0]!
  const SelectedIcon = selected.icon

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle className="text-sm">What {brand.Name} does</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="flex flex-wrap gap-1.5" role="tablist">
          {CAPABILITIES.map((capability) => {
            const ItemIcon = capability.icon
            const isSelected = capability.key === selectedKey
            return (
              <Button
                key={capability.key}
                type="button"
                role="tab"
                aria-selected={isSelected}
                variant={isSelected ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => {
                  setSelectedKey(capability.key)
                }}
              >
                <ItemIcon className="size-3.5" aria-hidden="true" />
                {capability.label}
              </Button>
            )
          })}
        </div>
        <div
          role="tabpanel"
          className="flex items-start gap-2.5 rounded-lg bg-muted/40 p-3"
        >
          <SelectedIcon
            className="mt-0.5 size-4 shrink-0 text-primary"
            aria-hidden="true"
          />
          <p className="text-sm text-muted-foreground">{selected.detail}</p>
        </div>
      </CardContent>
    </Card>
  )
}
