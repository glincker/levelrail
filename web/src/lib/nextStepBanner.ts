import type { SetupGuide } from './setupGuide'
import { isOpen } from './setupGuide'

// Order is priority: reach the app by name, then protect it, watch it, share it.
export type NextStepId =
  'app_domain' | 'first_backup' | 'notifications' | 'teammate'

export const NEXT_STEP_ORDER: readonly NextStepId[] = [
  'app_domain',
  'first_backup',
  'notifications',
  'teammate',
]

export interface NextStep {
  id: NextStepId
  href: string
}

// Picks the single banner an app shows after a successful deploy. The
// domain step is judged by this app's own domains, the rest by the
// platform setup guide so a finished or skipped step never nags.
export function pickNextStep(input: {
  appName: string
  hasDomain: boolean
  deploySucceeded: boolean
  guide: SetupGuide | undefined
  dismissed: ReadonlySet<string>
}): NextStep | null {
  if (!input.deploySucceeded || !input.guide) return null
  for (const id of NEXT_STEP_ORDER) {
    if (input.dismissed.has(id)) continue
    if (id === 'app_domain') {
      if (!input.hasDomain) {
        return {
          id,
          href: `/apps/${encodeURIComponent(input.appName)}/domains`,
        }
      }
      continue
    }
    const step = input.guide.steps.find((s) => s.id === id)
    if (step && isOpen(step)) return { id, href: step.href }
  }
  return null
}

const key = (app: string) => `lr-next-step-dismissed:${app}`

export function readNextStepDismissed(app: string): Set<string> {
  try {
    const parsed: unknown = JSON.parse(
      window.localStorage.getItem(key(app)) ?? '[]',
    )
    return new Set(
      Array.isArray(parsed)
        ? parsed.filter((v): v is string => typeof v === 'string')
        : [],
    )
  } catch {
    return new Set()
  }
}

export function writeNextStepDismissed(
  app: string,
  ids: ReadonlySet<string>,
): void {
  try {
    window.localStorage.setItem(key(app), JSON.stringify([...ids]))
  } catch {
    // storage unavailable: the dismissal lasts for this page view only
  }
}
