import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { StatusPill } from '@/components/kit'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Alert, AlertDescription } from '@/components/ui/alert'
import type { PromotePreviewResource } from '../types/promote'
import type { PromoteOptions } from '../lib/promoteOptions'

function KeyList({ label, keys }: { label: string; keys: string[] }) {
  if (keys.length === 0) return null
  return (
    <p className="text-xs text-muted-foreground">
      {label}:{' '}
      <span className="font-mono text-foreground">{keys.join(', ')}</span>
    </p>
  )
}

export function PromotePlan({
  preview,
  options,
  onChange,
}: {
  preview: PromotePreviewResource
  options: PromoteOptions
  onChange: (next: PromoteOptions) => void
}) {
  const d = preview.diff
  const config = [d?.replicas, d?.resources, d?.health].filter(
    (f): f is NonNullable<typeof f> => f !== undefined,
  )
  const envKeys = (d?.env_added.length ?? 0) + (d?.env_removed.length ?? 0)
  return (
    <div className="space-y-3">
      {config.length > 0 ? (
        <ul className="space-y-1 text-xs">
          {config.map((f) => (
            <li key={f.field} className="font-mono">
              <span className="text-muted-foreground">{f.field}: </span>
              {f.from} <span aria-hidden="true">&rarr;</span> {f.to}
            </li>
          ))}
        </ul>
      ) : null}
      {d ? (
        <div className="space-y-1">
          <KeyList label="Env keys to add" keys={d.env_added} />
          <KeyList label="Env keys to remove" keys={d.env_removed} />
          <KeyList
            label="Env keys with different values (kept)"
            keys={d.env_changed}
          />
          <KeyList
            label="Secret keys only on source"
            keys={d.secret_keys_added}
          />
          <KeyList
            label="Secret keys only on target"
            keys={d.secret_keys_removed}
          />
          <p className="text-xs text-muted-foreground">
            Left untouched: {d.untouched.join(', ')}. Values are never shown.
          </p>
        </div>
      ) : null}
      {envKeys > 0 ? (
        <label className="flex items-center gap-2 text-sm">
          <Checkbox
            checked={options.includeEnv}
            onCheckedChange={(checked) => {
              onChange({ ...options, includeEnv: checked })
            }}
          />
          Also apply the {envKeys} env key change{envKeys === 1 ? '' : 's'}
        </label>
      ) : null}
      {(preview.blockers?.length ?? 0) > 0 ? (
        <Alert variant="destructive">
          <WarningIcon className="size-4" />
          <AlertDescription className="space-y-2">
            {preview.blockers?.map((b) => (
              <p key={b}>{b}</p>
            ))}
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={options.force}
                onCheckedChange={(checked) => {
                  onChange({ ...options, force: checked })
                }}
              />
              Promote anyway
            </label>
          </AlertDescription>
        </Alert>
      ) : null}
      {preview.frozen ? (
        <Alert>
          <WarningIcon className="size-4" />
          <AlertDescription className="space-y-2">
            <p className="flex items-center gap-2">
              <StatusPill tone="warning" label="Deploy freeze" size="sm" />
              {preview.freeze_reason || 'A freeze window is active.'}
            </p>
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={options.overrideFreeze}
                onCheckedChange={(checked) => {
                  onChange({ ...options, overrideFreeze: checked })
                }}
              />
              Override the freeze
            </label>
            {options.overrideFreeze ? (
              <Input
                aria-label="Freeze override reason"
                placeholder="Reason (recorded on the deploy)"
                value={options.overrideReason}
                onChange={(e) => {
                  onChange({ ...options, overrideReason: e.target.value })
                }}
              />
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}
    </div>
  )
}
