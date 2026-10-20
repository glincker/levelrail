import { RobotIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from './ui/button'

// One toggle chip per known agent name. Selecting the active chip clears the
// filter. Renders nothing when there are no agents to filter by.
export function AgentFilterChips({
  agents,
  active,
  onChange,
}: {
  agents: string[]
  active: string | undefined
  onChange: (agent: string | undefined) => void
}) {
  if (agents.length === 0) {
    return null
  }
  return (
    <div
      role="group"
      aria-label="Filter by agent"
      className="flex flex-wrap items-center gap-1.5"
    >
      {agents.map((name) => {
        const selected = name === active
        return (
          <Button
            key={name}
            type="button"
            size="sm"
            variant={selected ? 'default' : 'outline'}
            aria-pressed={selected}
            onClick={() => {
              onChange(selected ? undefined : name)
            }}
          >
            <RobotIcon className="size-3.5" aria-hidden="true" />
            {name}
          </Button>
        )
      })}
    </div>
  )
}
