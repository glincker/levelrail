import { shallowRef } from 'vue'
import { charts, logs } from './mockData'
import type { MockChart, MockLog } from './mockData'

export interface LiveSeries {
  name: string
  tone: MockChart['series'][number]['tone']
  values: number[]
  area?: boolean
  mean: number
  amp: number
}
export interface LiveChart extends Omit<MockChart, 'series'> {
  series: LiveSeries[]
  live: boolean
}

const fmt: Record<string, (v: number) => string> = {
  cpu: (v) => `${v.toFixed(1)}%`,
  memory: (v) => `${Math.round(v)} MiB`,
  network: (v) => `${Math.round(v)} KiB/s`,
  disk: (v) => `${v.toFixed(1)} MiB/s`,
}

function seed(): LiveChart[] {
  return charts.map((c) => ({
    ...c,
    live: false,
    series: c.series.map((s) => {
      const lo = Math.min(...s.values)
      const hi = Math.max(...s.values)
      return {
        ...s,
        values: [s.values[0], ...s.values],
        mean: s.values.reduce((a, b) => a + b, 0) / s.values.length,
        amp: (hi - lo) / 5,
      }
    }),
  }))
}

export function useLiveCharts() {
  const state = shallowRef<LiveChart[]>(seed())

  function advance(): void {
    state.value = state.value.map((c) => {
      const series = c.series.map((s) => {
        const last = s.values[s.values.length - 1]
        const next = s.amp === 0 ? last : s.mean + (last - s.mean) * 0.75 + (Math.random() - 0.5) * s.amp * 1.4
        const clamped = Math.min(c.yMax, Math.max(c.yMin, next))
        return { ...s, values: [...s.values.slice(1), clamped] }
      })
      const main = series.find((s) => s.area) ?? series[0]
      return { ...c, series, live: true, value: fmt[c.id]?.(main.values[main.values.length - 1]) ?? c.value }
    })
  }

  return { charts: state, advance }
}

export const LOG_WINDOW = 22
const paths = ['/', '/pricing', '/docs/getting-started', '/assets/app.css', '/assets/app.js', '/api/health', '/blog']

export function useLiveLogs() {
  const lines = shallowRef<MockLog[]>(logs.slice(0, LOG_WINDOW))
  let n = logs.length
  let secs = 14 + Math.floor((LOG_WINDOW - 1) * 2.3)

  function advance(): void {
    secs += 2 + (n % 2)
    const time = `20:${String(3 + Math.floor(secs / 60)).padStart(2, '0')}:${String(secs % 60).padStart(2, '0')}`
    const line: MockLog = {
      time,
      level: 'info',
      text: `172.18.0.${2 + (n % 3)} GET ${paths[n % paths.length]} HTTP/1.1 200 ${(896 + n * 37) % 4096} ${4 + ((n * 7) % 23)}ms`,
    }
    n += 1
    lines.value = [...lines.value.slice(-(LOG_WINDOW - 1)), line]
  }

  return { lines, advance }
}
