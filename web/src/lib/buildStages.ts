// Types and pure maths for the build stage graph. The wire shape is
// GET /api/v1/apps/{name}/deploys/{deployId}/stages.

export const BUILD_STAGE_ORDER = [
  'source',
  'detect',
  'install',
  'build',
  'export',
  'push',
] as const

export type BuildStageName = (typeof BUILD_STAGE_ORDER)[number]
export type BuildStageStatus = 'running' | 'done' | 'failed'
export type BuildStageCache = 'hit' | 'partial' | 'miss'

export interface BuildStage {
  stage: BuildStageName
  status: BuildStageStatus
  started_at: string
  ended_at?: string
  duration_ms: number
  cache?: BuildStageCache
  vertices: number
  cached_vertices: number
  log_bytes: number
}

export interface BuildStagesResponse {
  deploy_id: string
  status: string
  wall_ms: number
  stages_ms: number
  stages: BuildStage[]
}

export function isBuildStageName(value: string): value is BuildStageName {
  return (BUILD_STAGE_ORDER as readonly string[]).includes(value)
}

// Lists are never null: null and a missing field both mean empty.
export function normalizeBuildStages(
  raw: Partial<BuildStagesResponse> | null | undefined,
): BuildStagesResponse {
  const stages = (raw?.stages ?? []).filter((s) => isBuildStageName(s.stage))
  stages.sort(
    (a, b) =>
      BUILD_STAGE_ORDER.indexOf(a.stage) - BUILD_STAGE_ORDER.indexOf(b.stage),
  )
  return {
    deploy_id: raw?.deploy_id ?? '',
    status: raw?.status ?? '',
    wall_ms: raw?.wall_ms ?? 0,
    stages_ms: raw?.stages_ms ?? 0,
    stages,
  }
}

function startMs(stage: BuildStage): number {
  const ms = Date.parse(stage.started_at)
  return Number.isFinite(ms) ? ms : 0
}

// A running stage has no ended_at, so its length is measured to now.
export function stageDurationMs(stage: BuildStage, nowMs: number): number {
  if (stage.status === 'running' && !stage.ended_at) {
    return Math.max(0, nowMs - startMs(stage))
  }
  if (stage.duration_ms > 0) return stage.duration_ms
  if (stage.ended_at) {
    const end = Date.parse(stage.ended_at)
    if (Number.isFinite(end)) return Math.max(0, end - startMs(stage))
  }
  return 0
}

export type CacheChipKind = 'hit' | 'partial' | 'miss' | 'none'

export interface CacheChipState {
  kind: CacheChipKind
  cached: number
  total: number
}

// Stages with no cacheable steps (source, detect) show no chip rather
// than a made up miss.
export function cacheChipState(stage: BuildStage): CacheChipState {
  const total = stage.vertices
  const cached = Math.min(stage.cached_vertices, total)
  if (total <= 0 || !stage.cache) return { kind: 'none', cached: 0, total: 0 }
  return { kind: stage.cache, cached, total }
}

export interface WaterfallRow {
  stage: BuildStageName
  status: BuildStageStatus
  offsetMs: number
  durationMs: number
  /** Left edge and width as percentages of the axis, clamped to 0..100. */
  leftPct: number
  widthPct: number
}

export interface Waterfall {
  rows: WaterfallRow[]
  /** Axis length: wall time when known, else the span the stages cover. */
  axisMs: number
}

function clampPct(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

export function buildWaterfall(
  data: BuildStagesResponse,
  nowMs: number,
): Waterfall {
  if (data.stages.length === 0) return { rows: [], axisMs: 0 }
  const origin = Math.min(...data.stages.map(startMs))
  const raw = data.stages.map((s) => {
    const offsetMs = Math.max(0, startMs(s) - origin)
    return { s, offsetMs, durationMs: stageDurationMs(s, nowMs) }
  })
  const span = Math.max(...raw.map((r) => r.offsetMs + r.durationMs))
  const axisMs = Math.max(data.wall_ms, span)
  const rows = raw.map(({ s, offsetMs, durationMs }) => ({
    stage: s.stage,
    status: s.status,
    offsetMs,
    durationMs,
    leftPct: axisMs > 0 ? clampPct((offsetMs / axisMs) * 100) : 0,
    widthPct: axisMs > 0 ? clampPct((durationMs / axisMs) * 100) : 0,
  }))
  return { rows, axisMs }
}

export type WallGap =
  | { kind: 'match' }
  | { kind: 'outside'; ms: number }
  | { kind: 'overlap'; ms: number }

// stages_ms is the sum of stage durations and wall_ms the elapsed
// time; they differ when stages overlap or time passes between them.
// A small tolerance keeps rounding from reading as a gap.
export function wallGap(
  data: BuildStagesResponse,
  toleranceMs = 1000,
): WallGap {
  if (data.wall_ms <= 0 || data.stages_ms <= 0) return { kind: 'match' }
  const diff = data.wall_ms - data.stages_ms
  if (Math.abs(diff) <= toleranceMs) return { kind: 'match' }
  return diff > 0
    ? { kind: 'outside', ms: diff }
    : { kind: 'overlap', ms: -diff }
}

export function failedStage(data: BuildStagesResponse): BuildStage | null {
  return data.stages.find((s) => s.status === 'failed') ?? null
}

export function hasRunningStage(data: BuildStagesResponse): boolean {
  return data.stages.some((s) => s.status === 'running' && !s.ended_at)
}

// The build is live while the deploy is queued or running, or any
// stage is still open. Polling stops as soon as none of these hold.
export function stagesPollInterval(
  data: BuildStagesResponse | undefined,
  pollMs: number,
): number | false {
  if (!data) return false
  if (data.status === 'running' || data.status === 'queued') return pollMs
  return hasRunningStage(data) ? pollMs : false
}

export function isBuildStepKey(key: string | undefined): boolean {
  return key?.startsWith('build_stage.') ?? false
}

// Elapsed time of the whole run: live while open, else the recorded wall time.
export function runElapsedMs(data: BuildStagesResponse, nowMs: number): number {
  const live = data.status === 'running' || data.status === 'queued'
  if (live && data.stages.length > 0) {
    const first = Math.min(...data.stages.map(startMs))
    return Math.max(0, nowMs - first)
  }
  return data.wall_ms > 0 ? data.wall_ms : data.stages_ms
}
