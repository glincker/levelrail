import type {
  DomainTuning,
  DomainTuningInput,
  TuningPreset,
} from '../../queries/domainTuning'

export const TUNING_PRESETS: readonly TuningPreset[] = [
  'api',
  'uploads',
  'streaming',
  'websocket',
]

type Bounds = Pick<
  DomainTuning,
  'min_timeout_seconds' | 'max_timeout_seconds' | 'max_stream_seconds'
>

// Preset seconds are starting points; the server bounds always win, so clamp first.
const PRESET_SECONDS = {
  apiResponse: 60,
  longResponse: 300,
  socketCloseDelay: 60,
  socketLifetime: 86_400,
} as const

function fit(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(v, hi))
}

// presetPayload maps a use case to the PUT body, mirroring the CLI's --preset.
export function presetPayload(
  preset: TuningPreset,
  b: Bounds,
): DomainTuningInput {
  const base: DomainTuningInput = {
    preset,
    compression: false,
    streaming: false,
    websocket: false,
    dial_timeout_seconds: 0,
    response_header_timeout_seconds: 0,
    stream_timeout_seconds: 0,
    stream_close_delay_seconds: 0,
  }
  const lo = b.min_timeout_seconds
  switch (preset) {
    case 'api':
      return {
        ...base,
        compression: true,
        response_header_timeout_seconds: fit(
          PRESET_SECONDS.apiResponse,
          lo,
          b.max_timeout_seconds,
        ),
      }
    case 'uploads':
      return {
        ...base,
        response_header_timeout_seconds: fit(
          PRESET_SECONDS.longResponse,
          lo,
          b.max_timeout_seconds,
        ),
      }
    case 'streaming':
      return {
        ...base,
        streaming: true,
        response_header_timeout_seconds: fit(
          PRESET_SECONDS.longResponse,
          lo,
          b.max_timeout_seconds,
        ),
      }
    case 'websocket':
      return {
        ...base,
        websocket: true,
        stream_close_delay_seconds: fit(
          PRESET_SECONDS.socketCloseDelay,
          lo,
          b.max_timeout_seconds,
        ),
        stream_timeout_seconds: fit(
          PRESET_SECONDS.socketLifetime,
          lo,
          b.max_stream_seconds,
        ),
      }
  }
}

export function isTuningPreset(v: string): v is TuningPreset {
  return (TUNING_PRESETS as readonly string[]).includes(v)
}
