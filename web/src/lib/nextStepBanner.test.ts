import { describe, expect, it } from 'vitest'
import { pickNextStep } from './nextStepBanner'
import type { SetupGuide, SetupGuideStepId } from './setupGuide'

function guide(open: SetupGuideStepId[]): SetupGuide {
  const ids: SetupGuideStepId[] = [
    'app_domain',
    'first_backup',
    'notifications',
    'teammate',
  ]
  return {
    steps: ids.map((id) => ({
      id,
      title: id,
      state: open.includes(id) ? 'todo' : 'done',
      optional: false,
      estimate_minutes: 1,
      href: `/step/${id}`,
    })),
    ready_percent: 0,
    done: 0,
    total: 4,
    complete: false,
    hidden: false,
  }
}

const base = { appName: 'web', hasDomain: false, deploySucceeded: true }

describe('pickNextStep', () => {
  it('shows nothing until a deploy has succeeded', () => {
    expect(
      pickNextStep({
        ...base,
        deploySucceeded: false,
        guide: guide(['teammate']),
        dismissed: new Set(),
      }),
    ).toBeNull()
  })

  it('shows one step at a time, domain first, then moves on when dismissed', () => {
    const g = guide(['first_backup', 'teammate'])
    const first = pickNextStep({ ...base, guide: g, dismissed: new Set() })
    expect(first).toEqual({ id: 'app_domain', href: '/apps/web/domains' })
    const second = pickNextStep({
      ...base,
      guide: g,
      dismissed: new Set(['app_domain']),
    })
    expect(second?.id).toBe('first_backup')
    expect(
      pickNextStep({
        ...base,
        hasDomain: true,
        guide: guide([]),
        dismissed: new Set(),
      }),
    ).toBeNull()
  })
})
