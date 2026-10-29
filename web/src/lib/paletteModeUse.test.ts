import { beforeEach, describe, expect, it } from 'vitest'
import { modesByUse, recordModeUse } from './paletteModeUse'

beforeEach(() => window.localStorage.clear())

describe('modesByUse', () => {
  it('starts in the default order', () => {
    expect(modesByUse()).toEqual(['docs', 'assistant', 'tags', 'docker'])
  })

  it('puts the most used mode first', () => {
    recordModeUse('docker')
    recordModeUse('docker')
    recordModeUse('tags')
    expect(modesByUse().slice(0, 2)).toEqual(['docker', 'tags'])
  })
})
