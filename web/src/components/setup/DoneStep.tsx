import type { ComponentType } from 'react'
import { Link } from '@tanstack/react-router'
import {
  BellRingingIcon,
  CheckCircleIcon,
  CloudArrowUpIcon,
  MinusCircleIcon,
  UsersIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { SETUP_STEPS, SETUP_STEP_META } from '../../lib/setupWizard'
import type { SetupStepId, SetupStepMap } from '../../lib/setupWizard'

const NEXT_STEPS: {
  to: string
  title: string
  description: string
  icon: ComponentType<{ className?: string }>
}[] = [
  {
    to: '/settings/notification-channels',
    title: 'Set up alerts',
    description:
      'Get a Slack, Discord, email, or webhook message when a deploy fails.',
    icon: BellRingingIcon,
  },
  {
    to: '/settings/backup-targets',
    title: 'Add a backup target',
    description: 'Point database and volume backups at S3-compatible storage.',
    icon: CloudArrowUpIcon,
  },
  {
    to: '/settings/users',
    title: 'Invite your team',
    description: 'Give teammates their own accounts instead of sharing yours.',
    icon: UsersIcon,
  },
]

/** DoneStep summarizes what was set up and links to the next things worth doing. */
export function DoneStep({
  steps,
  onGoToStep,
  onFinish,
  pending,
}: {
  steps: SetupStepMap
  onGoToStep: (id: SetupStepId) => void
  onFinish: () => void
  pending: boolean
}) {
  const reviewable = SETUP_STEPS.filter((id) => id !== 'done')

  return (
    <div className="space-y-5">
      <ul className="divide-y divide-border rounded-lg border border-border">
        {reviewable.map((id) => {
          const done = steps[id] === 'completed'
          return (
            <li
              key={id}
              className="flex items-center justify-between gap-3 p-3"
            >
              <span className="flex items-center gap-2 text-sm text-foreground">
                {done ? (
                  <CheckCircleIcon className="size-5 text-green-600 dark:text-green-400" />
                ) : (
                  <MinusCircleIcon className="size-5 text-muted-foreground" />
                )}
                {SETUP_STEP_META[id].title}
                <span className="text-xs text-muted-foreground">
                  {done ? 'Done' : 'Skipped'}
                </span>
              </span>
              {done ? null : (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => onGoToStep(id)}
                >
                  Go back
                </Button>
              )}
            </li>
          )
        })}
      </ul>

      <div className="space-y-2">
        <p className="text-sm font-medium text-foreground">Next steps</p>
        <div className="grid gap-2 sm:grid-cols-3">
          {NEXT_STEPS.map((n) => {
            const NextIcon = n.icon
            return (
              <Link
                key={n.to}
                to={n.to}
                className="space-y-1 rounded-lg border border-border p-3 transition-colors hover:bg-muted/50"
              >
                <NextIcon className="size-5 text-muted-foreground" />
                <p className="text-sm font-medium text-foreground">{n.title}</p>
                <p className="text-xs text-muted-foreground">{n.description}</p>
              </Link>
            )
          })}
        </div>
      </div>

      <div className="flex justify-end border-t border-border pt-4">
        <Button onClick={onFinish} disabled={pending}>
          Go to dashboard
        </Button>
      </div>
    </div>
  )
}
