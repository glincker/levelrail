import { HardDrivesIcon, StackIcon } from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import { StepFooter } from './StepChrome'
import type { StepProps } from './types'

/** TopologyStep states the single- vs multi-node tradeoff plainly; purely informational, never blocks. */
export function TopologyStep({ onContinue, onSkip, pending }: StepProps) {
  return (
    <div className="space-y-4">
      <p className="max-w-prose text-sm text-muted-foreground">
        Optional. This runs on a single node by default: nothing to decide now,
        and you can add a node later from Nodes in the sidebar. Here is the real
        tradeoff either way.
      </p>

      <div className="grid gap-3 sm:grid-cols-2">
        <div className="rounded-lg border border-border p-3">
          <p className="flex items-center gap-2 text-sm font-medium text-foreground">
            <HardDrivesIcon className="size-4 text-muted-foreground" />
            Single node
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            Simplest to run. One machine is a single point of failure, and
            builds share CPU and disk with whatever is already serving traffic
            there.
          </p>
        </div>
        <div className="rounded-lg border border-border p-3">
          <p className="flex items-center gap-2 text-sm font-medium text-foreground">
            <StackIcon className="size-4 text-muted-foreground" />
            Multi-node
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            Apps can run closer to users, and expensive builds can be routed to
            a node that isn&apos;t serving traffic. More to manage, and each
            node needs its own upkeep.
          </p>
        </div>
      </div>

      <p className="text-xs text-muted-foreground">
        <Link to="/nodes" className="text-primary underline underline-offset-4">
          Manage nodes
        </Link>{' '}
        any time, before or after finishing setup.
      </p>

      <StepFooter
        gate={{ canContinue: true }}
        onContinue={onContinue}
        onSkip={onSkip}
        continueLabel="Continue with single node"
        pending={pending}
      />
    </div>
  )
}
