import { SnowflakeIcon } from '@phosphor-icons/react/dist/ssr'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useDeployFreezeOptional } from '../queries/deployFreeze'
import type { DeploySafetyValues } from '../lib/imageDigest'

// DeploySafetyOptions holds a manual deploy's "pull fresh" choice and, only
// while the app is frozen, the required override reason.
export function DeploySafetyOptions({
  appName,
  values,
  onChange,
}: {
  appName: string
  values: DeploySafetyValues
  onChange: (next: DeploySafetyValues) => void
}) {
  const freeze = useDeployFreezeOptional(appName)
  const frozen = freeze.data?.status.frozen === true
  const until = freeze.data?.status.until

  return (
    <div className="mt-3 space-y-3">
      <div className="flex items-start gap-2">
        <Checkbox
          id={`deploy-pull-${appName}`}
          checked={values.pull}
          onCheckedChange={(checked) =>
            onChange({ ...values, pull: checked === true })
          }
        />
        <Label
          htmlFor={`deploy-pull-${appName}`}
          className="text-sm font-normal text-muted-foreground"
        >
          Require a fresh registry lookup: fail instead of deploying the cached
          image if the registry is unreachable.
        </Label>
      </div>
      {frozen ? (
        <div className="rounded-md border border-amber-300 bg-amber-50 p-3 dark:border-amber-800 dark:bg-amber-950/40">
          <p className="flex items-center gap-2 text-sm font-medium text-amber-900 dark:text-amber-200">
            <SnowflakeIcon className="size-4" />
            Deploy freeze active
            {until ? ` until ${new Date(until).toLocaleString()}` : ''}
          </p>
          <p className="mt-1 text-xs text-amber-900/80 dark:text-amber-200/80">
            A manual deploy now overrides the freeze. Say why; the reason is
            recorded on the deploy.
          </p>
          <Input
            className="mt-2"
            aria-label="Freeze override reason"
            placeholder="e.g. hotfix for a security issue"
            value={values.overrideReason}
            onChange={(e) =>
              onChange({ ...values, overrideReason: e.target.value })
            }
          />
        </div>
      ) : null}
    </div>
  )
}
