import { describe, expect, it } from 'vitest'
import { presetPayload } from './presets'

const bounds = {
  min_timeout_seconds: 1,
  max_timeout_seconds: 600,
  max_stream_seconds: 86_400,
}

describe('presetPayload', () => {
  it('api compresses and waits a minute for headers', () => {
    const p = presetPayload('api', bounds)
    expect(p).toMatchObject({
      preset: 'api',
      compression: true,
      streaming: false,
      websocket: false,
      response_header_timeout_seconds: 60,
      stream_timeout_seconds: 0,
    })
  })

  it('streaming flushes without compression', () => {
    const p = presetPayload('streaming', bounds)
    expect(p.streaming).toBe(true)
    expect(p.compression).toBe(false)
  })

  it('websocket sets bounded stream lifetimes only', () => {
    const p = presetPayload('websocket', bounds)
    expect(p).toMatchObject({
      websocket: true,
      compression: false,
      stream_close_delay_seconds: 60,
      stream_timeout_seconds: 86_400,
      response_header_timeout_seconds: 0,
    })
  })

  it('clamps every number to the live server bounds', () => {
    const tight = {
      min_timeout_seconds: 90,
      max_timeout_seconds: 120,
      max_stream_seconds: 3600,
    }
    expect(presetPayload('api', tight).response_header_timeout_seconds).toBe(90)
    expect(
      presetPayload('uploads', tight).response_header_timeout_seconds,
    ).toBe(120)
    const ws = presetPayload('websocket', tight)
    expect(ws.stream_close_delay_seconds).toBe(90)
    expect(ws.stream_timeout_seconds).toBe(3600)
  })
})
