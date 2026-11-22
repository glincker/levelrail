import { describe, expect, it } from 'vitest'
import { FORM_RECIPES, recipeChannelIds } from './formRecipes'
import type { NotificationChannel } from '../types/notificationChannel'

const ch = (id: string, kind: NotificationChannel['kind'], enabled = true) =>
  ({ id, kind, enabled }) as NotificationChannel

describe('form recipes', () => {
  it('wires the feedback recipe to an enabled Slack channel only', () => {
    const feedback = FORM_RECIPES.find((r) => r.id === 'feedback')!
    const list = [
      ch('e1', 'email'),
      ch('s0', 'slack', false),
      ch('s1', 'slack'),
    ]
    expect(recipeChannelIds(feedback, list)).toEqual(['s1'])
    expect(recipeChannelIds(feedback, [ch('e1', 'email')])).toEqual([])
  })

  it('every recipe has a valid form name and at least one field', () => {
    for (const r of FORM_RECIPES) {
      expect(r.name).toMatch(/^[a-z0-9][a-z0-9_-]{0,62}$/)
      expect(r.schema.fields.length).toBeGreaterThan(0)
    }
  })
})
