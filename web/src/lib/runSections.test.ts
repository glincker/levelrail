import { beforeEach, describe, expect, it } from 'vitest'
import {
  readSectionOpen,
  sectionFromHash,
  writeSectionOpen,
} from './runSections'
import { runElapsedMs, type BuildStagesResponse } from './buildStages'

describe('runSections', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('maps hashes and line permalinks to sections', () => {
    expect(sectionFromHash('#logs')).toBe('logs')
    expect(sectionFromHash('#waterfall')).toBe('waterfall')
    expect(sectionFromHash('#L120')).toBe('logs')
    expect(sectionFromHash('#nope')).toBeNull()
  })

  it('remembers open state and falls back to the default', () => {
    expect(readSectionOpen('summary', true)).toBe(true)
    writeSectionOpen('summary', false)
    expect(readSectionOpen('summary', true)).toBe(false)
  })
})

describe('runElapsedMs', () => {
  const base: BuildStagesResponse = {
    deploy_id: 'd',
    status: 'running',
    wall_ms: 0,
    stages_ms: 0,
    stages: [
      {
        stage: 'source',
        status: 'running',
        started_at: '2026-11-15T10:00:00Z',
        duration_ms: 0,
        vertices: 0,
        cached_vertices: 0,
        log_bytes: 0,
      },
    ],
  }
  it('counts up while running and uses wall time once done', () => {
    expect(runElapsedMs(base, Date.parse('2026-11-15T10:00:05Z'))).toBe(5000)
    expect(runElapsedMs({ ...base, status: 'success', wall_ms: 7000 }, 0)).toBe(
      7000,
    )
  })
})
