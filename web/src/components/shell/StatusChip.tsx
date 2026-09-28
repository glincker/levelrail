import { useNavigate } from '@tanstack/react-router'
import {
  BellSlashIcon,
  CheckCircleIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { StatusPill, type Tone } from '@/components/kit'
import { CleanUpDockerDialog } from '../CleanUpDockerDialog'
import {
  summarizeIssues,
  type ChipLevel,
  type PlatformIssue,
} from './platformIssues'
import { SNOOZE_LABELS, type SnoozeDuration } from './snooze'
import { snoozeNow, unsnoozeNow, useSnoozed } from './snoozeStore'
import { usePlatformIssues } from './usePlatformIssues'

const LEVEL_TONE: Record<ChipLevel, Tone> = {
  ok: 'success',
  warning: 'warning',
  critical: 'danger',
}

const DURATIONS: SnoozeDuration[] = ['1h', '1d', 'forever']

function IssueRow({ issue }: { issue: PlatformIssue }) {
  const navigate = useNavigate()
  const Icon = issue.severity === 'critical' ? WarningCircleIcon : WarningIcon
  return (
    <li className="flex items-start gap-2.5 rounded-lg px-2 py-2">
      <Icon
        aria-hidden="true"
        className={`mt-0.5 size-4 shrink-0 ${
          issue.severity === 'critical'
            ? 'text-destructive'
            : 'text-amber-600 dark:text-amber-400'
        }`}
      />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">{issue.title}</p>
        <p className="truncate text-xs text-muted-foreground">{issue.detail}</p>
        <div className="mt-1.5 flex items-center gap-1.5">
          {issue.actions.map((a) =>
            a.kind === 'cleanup-docker' ? (
              <CleanUpDockerDialog key="cleanup" />
            ) : (
              <Button
                key={a.label}
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void navigate({ to: a.to, hash: a.hash })}
              >
                {a.label}
              </Button>
            ),
          )}
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-label={`Snooze ${issue.title}`}
                />
              }
            >
              <BellSlashIcon />
              Snooze
            </DropdownMenuTrigger>
            <DropdownMenuContent>
              {DURATIONS.map((d) => (
                <DropdownMenuItem
                  key={d}
                  onClick={() => snoozeNow(issue.id, d)}
                >
                  {SNOOZE_LABELS[d]}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </li>
  )
}

export function StatusChip() {
  const issues = usePlatformIssues()
  const { isSnoozed } = useSnoozed()
  const { level, visible, snoozed } = summarizeIssues(issues, isSnoozed)
  const label =
    level === 'ok'
      ? 'All good'
      : `${String(visible.length)} ${visible.length === 1 ? 'issue' : 'issues'}`

  return (
    <Popover>
      <PopoverTrigger
        render={
          <button
            type="button"
            aria-label={`Platform status: ${label}`}
            className="rounded-full outline-hidden focus-visible:ring-2 focus-visible:ring-ring"
          />
        }
      >
        <StatusPill
          tone={LEVEL_TONE[level]}
          label={label}
          live={level === 'critical'}
          size="md"
        />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96">
        <PopoverHeader>
          <PopoverTitle>Platform status</PopoverTitle>
        </PopoverHeader>
        {visible.length === 0 ? (
          <div className="flex flex-col items-center gap-1.5 px-2 py-6 text-center">
            <CheckCircleIcon
              aria-hidden="true"
              className="size-6 text-emerald-600 dark:text-emerald-400"
            />
            <p className="text-sm text-muted-foreground">
              Docker, disk, nodes and certificates look fine.
            </p>
          </div>
        ) : (
          <ul className="flex flex-col">
            {visible.map((i) => (
              <IssueRow key={i.id} issue={i} />
            ))}
          </ul>
        )}
        {snoozed.length > 0 ? (
          <div className="mt-1 flex items-center justify-between border-t border-border px-2 pt-2 text-xs text-muted-foreground">
            <span>{snoozed.length} snoozed</span>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => snoozed.forEach((i) => unsnoozeNow(i.id))}
            >
              Show again
            </Button>
          </div>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}
