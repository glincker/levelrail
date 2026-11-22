import { describe, expect, it } from 'vitest'
import {
  buildWaterfall,
  cacheChipState,
  firstCacheMiss,
  normalizeBuildStages,
  stageDurationMs,
  stagesPollInterval,
  wallGap,
  type BuildStage,
  type BuildStagesResponse,
} from './buildStages'

const T0 = Date.parse('2026-11-15T10:00:00Z')
const iso = (offsetMs: number) => new Date(T0 + offsetMs).toISOString()

function stage(p: Partial<BuildStage> & Pick<BuildStage, 'stage'>): BuildStage {
  return {
    status: 'done',
    started_at: iso(0),
    ended_at: iso(1000),
    duration_ms: 1000,
    vertices: 0,
    cached_vertices: 0,
    log_bytes: 0,
    ...p,
  }
}

function data(p: Partial<BuildStagesResponse>): BuildStagesResponse {
  return {
    deploy_id: 'd1',
    status: 'success',
    wall_ms: 0,
    stages_ms: 0,
    stages: [],
    ...p,
  }
}

describe('normalizeBuildStages', () => {
  it('treats null and missing lists as empty', () => {
    expect(normalizeBuildStages(null).stages).toEqual([])
    expect(normalizeBuildStages({ stages: null as never }).stages).toEqual([])
  })

  it('orders stages in pipeline order and drops unknown names', () => {
    const out = normalizeBuildStages({
      stages: [
        stage({ stage: 'push' }),
        { ...stage({ stage: 'build' }), stage: 'weird' as never },
        stage({ stage: 'source' }),
      ],
    })
    expect(out.stages.map((s) => s.stage)).toEqual(['source', 'push'])
  })
})

describe('stageDurationMs', () => {
  it('measures a running stage with no ended_at to now', () => {
    const s = stage({
      stage: 'build',
      status: 'running',
      ended_at: undefined,
      duration_ms: 0,
    })
    expect(stageDurationMs(s, T0 + 4500)).toBe(4500)
    expect(stageDurationMs(s, T0 - 10)).toBe(0)
  })

  it('uses duration_ms for a finished stage, else the timestamps', () => {
    expect(
      stageDurationMs(stage({ stage: 'build', duration_ms: 2500 }), 0),
    ).toBe(2500)
    expect(stageDurationMs(stage({ stage: 'build', duration_ms: 0 }), 0)).toBe(
      1000,
    )
  })
})

describe('cacheChipState', () => {
  it('maps hit, partial with N of M, and miss', () => {
    expect(
      cacheChipState(
        stage({
          stage: 'install',
          cache: 'hit',
          vertices: 4,
          cached_vertices: 4,
        }),
      ),
    ).toEqual({ kind: 'hit', cached: 4, total: 4 })
    expect(
      cacheChipState(
        stage({
          stage: 'build',
          cache: 'partial',
          vertices: 9,
          cached_vertices: 7,
        }),
      ),
    ).toEqual({ kind: 'partial', cached: 7, total: 9 })
    expect(
      cacheChipState(stage({ stage: 'build', cache: 'miss', vertices: 3 }))
        .kind,
    ).toBe('miss')
  })

  it('shows no chip for a stage without cacheable steps', () => {
    expect(cacheChipState(stage({ stage: 'source' })).kind).toBe('none')
  })
})

describe('buildWaterfall', () => {
  it('sizes bars against wall time', () => {
    const wf = buildWaterfall(
      data({
        wall_ms: 10000,
        stages_ms: 6000,
        stages: [
          stage({ stage: 'source', duration_ms: 1000 }),
          stage({
            stage: 'build',
            started_at: iso(4000),
            ended_at: iso(9000),
            duration_ms: 5000,
          }),
        ],
      }),
      T0,
    )
    expect(wf.axisMs).toBe(10000)
    expect(wf.rows[0]).toMatchObject({ leftPct: 0, widthPct: 10 })
    expect(wf.rows[1]).toMatchObject({ leftPct: 40, widthPct: 50 })
  })

  it('grows a running stage to now and never exceeds 100 percent', () => {
    const wf = buildWaterfall(
      data({
        status: 'running',
        wall_ms: 2000,
        stages: [
          stage({
            stage: 'build',
            status: 'running',
            ended_at: undefined,
            duration_ms: 0,
          }),
        ],
      }),
      T0 + 8000,
    )
    expect(wf.axisMs).toBe(8000)
    expect(wf.rows[0]?.widthPct).toBe(100)
  })

  it('is empty without stages', () => {
    expect(buildWaterfall(data({}), T0)).toEqual({ rows: [], axisMs: 0 })
  })
})

describe('wallGap', () => {
  it('reports time outside stages, overlap, or a match', () => {
    expect(wallGap(data({ wall_ms: 10000, stages_ms: 6000 }))).toEqual({
      kind: 'outside',
      ms: 4000,
    })
    expect(wallGap(data({ wall_ms: 5000, stages_ms: 8000 }))).toEqual({
      kind: 'overlap',
      ms: 3000,
    })
    expect(wallGap(data({ wall_ms: 5200, stages_ms: 5000 })).kind).toBe('match')
    expect(wallGap(data({ wall_ms: 0, stages_ms: 0 })).kind).toBe('match')
  })
})

describe('stagesPollInterval', () => {
  it('does not poll before data arrives', () => {
    expect(stagesPollInterval(undefined, 3000)).toBe(false)
  })

  it('polls while the deploy runs or a stage is open, then stops', () => {
    expect(stagesPollInterval(data({ status: 'running' }), 3000)).toBe(3000)
    expect(
      stagesPollInterval(
        data({
          status: 'success',
          stages: [
            stage({ stage: 'build', status: 'running', ended_at: undefined }),
          ],
        }),
        3000,
      ),
    ).toBe(3000)
    expect(
      stagesPollInterval(
        data({ status: 'success', stages: [stage({ stage: 'build' })] }),
        3000,
      ),
    ).toBe(false)
    expect(stagesPollInterval(data({ status: 'failed' }), 3000)).toBe(false)
  })
})

describe('firstCacheMiss', () => {
  it('returns the first stage that names a missed step', () => {
    const d = data({
      stages: [
        stage({ stage: 'source' }),
        stage({
          stage: 'install',
          miss_step: '[3/5] RUN npm ci',
          miss_cause: 'copy_before_install',
        }),
      ],
    })
    expect(firstCacheMiss(d)).toEqual({
      stage: 'install',
      step: '[3/5] RUN npm ci',
      cause: 'copy_before_install',
    })
    expect(
      firstCacheMiss(data({ stages: [stage({ stage: 'build' })] })),
    ).toBeNull()
  })
})
