import { useState } from 'react'
import { FlagIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { FeatureFlagDialog } from './FeatureFlagDialog'
import { DeleteFeatureFlagDialog } from './DeleteFeatureFlagDialog'
import { useFeatureFlags, useUpdateFeatureFlag } from '../queries/featureFlags'
import type { FeatureFlag } from '../types/featureFlags'

// EnabledToggle flips a flag's Enabled kill switch in place through the
// same PUT .../flags/{id} the edit dialog uses, resubmitting its current
// Name/Description/RolloutPercentage unchanged: there is no separate
// PATCH-just-enabled route, matching EnabledToggle's own precedent in
// ScheduledTasksPanel. Every caller of GET /api/v1/flags/evaluate/{key}
// sees the new value on its very next call, no redeploy involved.
function EnabledToggle({ appName, flag }: { appName: string; flag: FeatureFlag }) {
  const updateFlag = useUpdateFeatureFlag(appName)

  return (
    <Switch
      checked={flag.enabled}
      disabled={updateFlag.isPending}
      aria-label={`${flag.enabled ? 'Disable' : 'Enable'} ${flag.name}`}
      onCheckedChange={(checked) => {
        updateFlag.mutate(
          {
            id: flag.id,
            req: {
              name: flag.name,
              description: flag.description ?? '',
              enabled: checked,
              rollout_percentage: flag.rollout_percentage,
            },
          },
          {
            onError: (error) => {
              toast.add({ title: error.message, type: 'error' })
            },
          },
        )
      }}
    />
  )
}

// RolloutInput edits rollout_percentage in place, committed on blur (not
// on every keystroke) so a caller mid-typing a two-digit number doesn't
// fire a request per digit. Same live-effect immediately shape as
// EnabledToggle above.
function RolloutInput({ appName, flag }: { appName: string; flag: FeatureFlag }) {
  const updateFlag = useUpdateFeatureFlag(appName)
  const [value, setValue] = useState(String(flag.rollout_percentage))

  function commit() {
    const parsed = Math.min(100, Math.max(0, Number(value) || 0))
    setValue(String(parsed))
    if (parsed === flag.rollout_percentage) {
      return
    }
    updateFlag.mutate(
      {
        id: flag.id,
        req: {
          name: flag.name,
          description: flag.description ?? '',
          enabled: flag.enabled,
          rollout_percentage: parsed,
        },
      },
      {
        onError: (error) => {
          setValue(String(flag.rollout_percentage))
          toast.add({ title: error.message, type: 'error' })
        },
      },
    )
  }

  return (
    <div className="flex items-center gap-1">
      <Input
        type="number"
        min={0}
        max={100}
        step={1}
        className="h-8 w-16"
        value={value}
        disabled={updateFlag.isPending}
        aria-label={`Rollout percentage for ${flag.name}`}
        onChange={(e) => setValue(e.target.value)}
        onBlur={commit}
      />
      <span className="text-xs text-muted-foreground">%</span>
    </div>
  )
}

function FlagRow({ appName, flag }: { appName: string; flag: FeatureFlag }) {
  return (
    <TableRow>
      <TableCell className="font-mono text-xs text-foreground">{flag.key}</TableCell>
      <TableCell className="text-sm text-foreground">{flag.name}</TableCell>
      <TableCell>
        <EnabledToggle appName={appName} flag={flag} />
      </TableCell>
      <TableCell>
        <RolloutInput appName={appName} flag={flag} />
      </TableCell>
      <TableCell className="text-right">
        <div className="flex justify-end gap-2">
          <FeatureFlagDialog appName={appName} flag={flag} />
          <DeleteFeatureFlagDialog appName={appName} flag={flag} />
        </div>
      </TableCell>
    </TableRow>
  )
}

export function FeatureFlagsPanel({ appName }: { appName: string }) {
  const { data, isLoading, error } = useFeatureFlags(appName)
  const flags = data ?? []

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <FlagIcon className="size-4 text-muted-foreground" aria-hidden="true" />
            Feature flags
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Read live by this app&apos;s own code via a bearer-token API call.
            Toggling enabled or the rollout percentage here takes effect
            immediately, no redeploy or restart needed.
          </p>
        </div>
        <FeatureFlagDialog appName={appName} />
      </div>

      <div className="mt-3">
        {isLoading ? (
          <TableSkeleton columnCount={5} rowCount={3} />
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : flags.length === 0 ? (
          <EmptyState
            icon={<FlagIcon className="size-5" />}
            title="No feature flags yet"
            description="Create a flag, then call GET /api/v1/flags/evaluate/{key} from your app's own code with a read-scoped API token to read its live value."
            action={<FeatureFlagDialog appName={appName} />}
          />
        ) : (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Key</TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Enabled</TableHead>
                  <TableHead>Rollout</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {flags.map((flag) => (
                  <FlagRow key={flag.id} appName={appName} flag={flag} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </section>
  )
}
