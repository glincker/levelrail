import { afterEach, describe, expect, it } from 'vitest'
import { getTeamScope, setTeamScope } from './teamScope'

describe('teamScope', () => {
  afterEach(() => {
    setTeamScope({ team: '', project: '' })
  })

  it('round trips team and project through cookies the server reads', () => {
    setTeamScope({ team: 'org_a', project: 'proj 1' })
    expect(getTeamScope()).toEqual({ team: 'org_a', project: 'proj 1' })
    expect(document.cookie).toContain('scope_team=org_a')
    expect(document.cookie).toContain('scope_project=proj%201')
  })

  it('clears both cookies for all teams', () => {
    setTeamScope({ team: 'org_a', project: '' })
    setTeamScope({ team: '', project: '' })
    expect(getTeamScope()).toEqual({ team: '', project: '' })
    expect(document.cookie).not.toContain('scope_team=org_a')
  })

  it('returns a stable object while the cookies are unchanged', () => {
    setTeamScope({ team: 'org_b', project: '' })
    expect(getTeamScope()).toBe(getTeamScope())
  })
})
