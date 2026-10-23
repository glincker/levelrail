import { beforeEach, describe, expect, it } from 'vitest'
import {
  getProjectScope,
  resetProjectScopeForTests,
  setProjectScope,
} from './projectScope'

describe('projectScope', () => {
  beforeEach(() => {
    window.localStorage.clear()
    resetProjectScopeForTests()
  })

  it('stores the chosen project and clears it for all projects', () => {
    setProjectScope('p1')
    expect(getProjectScope()).toBe('p1')
    expect(window.localStorage.getItem('project.scope.v1')).toBe('p1')
    expect(new URLSearchParams(window.location.search).get('project')).toBe(
      'p1',
    )
    setProjectScope('')
    expect(getProjectScope()).toBe('')
    expect(window.localStorage.getItem('project.scope.v1')).toBeNull()
  })
})
