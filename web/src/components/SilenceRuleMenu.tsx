import { BellSlashIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { toast } from '@/components/ui/toast'
import { useSilenceRule } from '../queries/alertNoise'
import { QUICK_SILENCE_DURATIONS } from '../types/alertNoise'

// Quick action: mute one rule's notifications for 1h, 4h or 24h. The
// rule keeps evaluating and shows up in history as silenced.
export function SilenceRuleMenu({
  appName,
  ruleId,
  ruleName,
}: {
  appName: string
  ruleId: string
  ruleName: string
}) {
  const silence = useSilenceRule(appName)

  function run(duration: string) {
    silence.mutate(
      { ruleId, duration },
      {
        onSuccess: () => {
          toast.add({
            title: `"${ruleName}" silenced for ${duration}.`,
            type: 'success',
          })
        },
        onError: (err) => {
          toast.add({ title: err.message, type: 'error' })
        },
      },
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={silence.isPending}
            aria-label={`Silence ${ruleName}`}
          />
        }
      >
        <BellSlashIcon className="size-3.5" aria-hidden="true" />
        Silence
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        {QUICK_SILENCE_DURATIONS.map((d) => (
          <DropdownMenuItem
            key={d}
            onClick={() => {
              run(d)
            }}
          >
            Silence for {d}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
