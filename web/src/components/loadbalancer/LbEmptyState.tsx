import { ArrowRightIcon, ArrowsSplitIcon } from '@phosphor-icons/react/dist/ssr'
import { EmptyState } from '@/components/kit'
import { Button } from '@/components/ui/button'
import {
  LB_PRESETS,
  presetById,
  RECOMMENDED_PRESET_ID,
  type LbPreset,
} from './presets'

function Diagram({ replicas }: { replicas: number }) {
  const shown = Math.max(replicas, 2)
  return (
    <div
      aria-label={`Recommended setup: clients, proxy, ${shown} replicas`}
      role="img"
      className="flex flex-wrap items-center justify-center gap-2 text-xs"
    >
      <span className="rounded-lg border px-2.5 py-1">Clients</span>
      <ArrowRightIcon className="size-3.5 text-muted-foreground" aria-hidden />
      <span className="rounded-lg border border-tone-accent-border bg-tone-accent-soft px-2.5 py-1">
        Proxy
      </span>
      <ArrowRightIcon className="size-3.5 text-muted-foreground" aria-hidden />
      <span className="flex gap-1">
        {Array.from({ length: shown }, (_, i) => (
          <span
            key={i}
            className="rounded-lg border px-2 py-1 text-muted-foreground"
          >
            R{i}
          </span>
        ))}
      </span>
    </div>
  )
}

export function LbEmptyState({
  replicas,
  creating,
  onCreate,
  onPreset,
}: {
  replicas: number
  creating: boolean
  onCreate: (preset: LbPreset) => void
  onPreset: (preset: LbPreset) => void
}) {
  const recommended = presetById(RECOMMENDED_PRESET_ID)
  return (
    <div className="space-y-6">
      <EmptyState
        icon={<ArrowsSplitIcon />}
        title="Spread traffic across replicas"
        description="Health checks, retries and clean cutovers. Least connections with passive failure detection is a good start. Add an active health check once your app serves one."
        action={
          recommended ? (
            <Button
              type="button"
              disabled={creating}
              onClick={() => onCreate(recommended)}
            >
              {creating ? 'Creating' : 'Create recommended setup'}
            </Button>
          ) : undefined
        }
      />
      <Diagram replicas={replicas} />
      <div>
        <h2 className="mb-2 text-sm font-medium">Or start from a preset</h2>
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {LB_PRESETS.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => onPreset(p)}
                className="w-full rounded-xl border p-3 text-left outline-none transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring/60"
              >
                <span className="block text-sm font-medium">{p.name}</span>
                <span className="block text-xs text-muted-foreground">
                  {p.useWhen}
                </span>
              </button>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
