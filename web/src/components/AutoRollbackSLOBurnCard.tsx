import { ArrowCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { HelpLink } from '@/components/HelpLink'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useAutoRollbackSLOBurn,
  useSetAutoRollbackSLOBurn,
  type SLOBurnAutoRollbackMode,
} from '../queries/autoRollbackSLOBurn'

const MODE_OPTIONS: {
  value: SLOBurnAutoRollbackMode
  label: string
  description: string
}[] = [
  {
    value: 'off',
    label: 'Off',
    description: 'Only alert; never act automatically.',
  },
  {
    value: 'auto',
    label: 'Auto rollback',
    description:
      'Roll back immediately to the most recent successful image older than the one burning the error budget.',
  },
  {
    value: 'dry_run',
    label: 'Dry run',
    description:
      'Log what would have been rolled back, without ever deploying.',
  },
  {
    value: 'pause_for_human',
    label: 'Pause for human',
    description:
      'Open a pending deploy approval for the rollback instead of deploying directly; a teammate approves or rejects it.',
  },
]

// AutoRollbackSLOBurnCard is the opt-in mode selector for
// internal/alerting.MaybeAutoRollbackOnSLOBurn (GET/PUT
// /api/v1/apps/{name}/auto-rollback-slo-burn): off by default, the same
// risky-by-default-feature-is-opt-in shape AutoRollbackCard's own
// crashloop toggle already establishes, but a mode selector rather than a
// switch since there are three distinct reactions to choose between, not
// just on/off. Rendered alongside AutoRollbackCard on the Deploys route.
export function AutoRollbackSLOBurnCard({ appName }: { appName: string }) {
  const setting = useAutoRollbackSLOBurn(appName)
  const setMode = useSetAutoRollbackSLOBurn(appName)

  function change(next: SLOBurnAutoRollbackMode | null) {
    if (!next || next === setting.data.mode) return
    setMode.mutate(next, {
      onSuccess: () => {
        const label = MODE_OPTIONS.find((o) => o.value === next)?.label ?? next
        toast.add({
          title: `Auto-rollback on SLO burn set to: ${label}.`,
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({
          title: 'Could not update auto-rollback on SLO burn.',
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const current = MODE_OPTIONS.find((o) => o.value === setting.data.mode)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ArrowCounterClockwiseIcon className="size-4 text-muted-foreground" />
          Auto-rollback on SLO burn
          <HelpLink
            path="/observability#alert-rules"
            label="Auto-rollback guide"
          />
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-sm font-medium text-foreground">Mode</p>
            <p className="text-sm text-muted-foreground">
              {current?.description ??
                'Choose how this app reacts when an SLO burn-rate alert fires.'}
            </p>
          </div>
          <Select
            value={setting.data.mode}
            onValueChange={change}
            disabled={setMode.isPending}
          >
            <SelectTrigger
              className="w-44"
              aria-label="Auto-rollback on SLO burn mode"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {MODE_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </CardContent>
    </Card>
  )
}
