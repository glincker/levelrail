import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  CircleIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import type { AppDetail } from '../../types/appDetail'
import { computeSetup, type SetupItem } from './suggestions'

const RADIUS = 8
const CIRCUMFERENCE = 2 * Math.PI * RADIUS

const TARGET: Record<SetupItem['id'], string> = {
  domain: '/apps/$name/domains',
  health: '/apps/$name/health',
  git: '/apps/$name/source',
  limits: '/apps/$name/resources',
}

const storageKey = (app: string) => `overview.setup.dismissed.${app}`

function readDismissed(app: string): boolean {
  try {
    return localStorage.getItem(storageKey(app)) === '1'
  } catch {
    return false
  }
}

export function SetupRing({
  app,
  hasGitSource,
}: {
  app: Pick<AppDetail, 'name' | 'health' | 'resources' | 'domains'>
  hasGitSource: boolean
}) {
  const [dismissed, setDismissed] = useState(() => readDismissed(app.name))
  const { items, done } = computeSetup({ app, hasGitSource })
  if (dismissed || done === items.length) return null

  const dismiss = () => {
    setDismissed(true)
    try {
      localStorage.setItem(storageKey(app.name), '1')
    } catch {
      // storage unavailable: dismissal lasts until reload
    }
  }

  return (
    <Popover>
      <PopoverTrigger className="inline-flex items-center gap-2 rounded-full border border-border px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring">
        <svg width="20" height="20" viewBox="0 0 20 20" aria-hidden="true">
          <circle
            cx="10"
            cy="10"
            r={RADIUS}
            fill="none"
            strokeWidth="2"
            className="stroke-muted"
          />
          <circle
            cx="10"
            cy="10"
            r={RADIUS}
            fill="none"
            strokeWidth="2"
            strokeLinecap="round"
            transform="rotate(-90 10 10)"
            strokeDasharray={CIRCUMFERENCE}
            strokeDashoffset={CIRCUMFERENCE * (1 - done / items.length)}
            className="stroke-primary"
          />
        </svg>
        Setup {done}/{items.length}
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64">
        <ul className="space-y-1">
          {items.map((i) => (
            <li key={i.id}>
              {i.done ? (
                <span className="flex items-center gap-2 px-1 py-1 text-muted-foreground line-through">
                  <CheckCircleIcon
                    className="size-4 text-green-600"
                    aria-hidden="true"
                  />
                  {i.label}
                </span>
              ) : (
                <Link
                  to={TARGET[i.id]}
                  params={{ name: app.name }}
                  className="flex items-center gap-2 rounded-md px-1 py-1 hover:bg-muted"
                >
                  <CircleIcon className="size-4" aria-hidden="true" />
                  {i.label}
                </Link>
              )}
            </li>
          ))}
        </ul>
        <button
          type="button"
          onClick={dismiss}
          className="flex items-center gap-1 self-start text-xs text-muted-foreground hover:text-foreground"
        >
          <XIcon className="size-3" aria-hidden="true" />
          Hide setup
        </button>
      </PopoverContent>
    </Popover>
  )
}
