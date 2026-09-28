import { CheckCircleIcon, CircleIcon } from '@phosphor-icons/react/dist/ssr'
import type { ControlPlaneDrChecklist } from '../queries/controlPlaneDr'

const STEPS: { key: keyof ControlPlaneDrChecklist; label: string }[] = [
  { key: 'destination_chosen', label: 'Choose a storage destination' },
  { key: 'recipient_set', label: 'Add at least one age public key' },
  {
    key: 'escrow_acknowledged',
    label: 'Download the escrow bundle and store it offline',
  },
  { key: 'drill_passed', label: 'Pass a restore drill' },
]

export function ControlPlaneDrChecklistView({
  checklist,
}: {
  checklist: ControlPlaneDrChecklist
}) {
  const done = STEPS.filter((s) => checklist[s.key]).length
  return (
    <div>
      <p className="mb-2 text-sm font-medium text-foreground">
        Set up disaster recovery ({done} of {STEPS.length})
      </p>
      <ol className="space-y-1.5">
        {STEPS.map((s) => {
          const ok = checklist[s.key]
          const Icon = ok ? CheckCircleIcon : CircleIcon
          return (
            <li key={s.key} className="flex items-center gap-2 text-sm">
              <Icon
                weight={ok ? 'fill' : 'regular'}
                className={
                  ok
                    ? 'size-4 shrink-0 text-emerald-600'
                    : 'size-4 shrink-0 text-muted-foreground'
                }
                aria-hidden="true"
              />
              <span
                className={ok ? 'text-foreground' : 'text-muted-foreground'}
              >
                {s.label}
              </span>
              <span className="sr-only">{ok ? 'done' : 'not done'}</span>
            </li>
          )
        })}
      </ol>
    </div>
  )
}
