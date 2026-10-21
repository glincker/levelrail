import { describe, expect, it } from 'vitest'
import type { LogLine } from '../hooks/useLogStream'
import { parseBuildCacheStats } from './buildCacheStats'

function mk(texts: string[]): LogLine[] {
  return texts.map((line, id) => ({ id, line, stream: 'stdout' as const }))
}

describe('parseBuildCacheStats', () => {
  it('counts cached steps against all numbered steps', () => {
    const stats = parseBuildCacheStats(
      mk([
        '#1 [internal] load build definition',
        '#5 [2/4] WORKDIR /app',
        '#5 CACHED',
        '#6 [3/4] RUN npm ci',
        '#6 0.5 added 10 packages',
        '#7 [4/4] COPY . .',
        '#7 CACHED',
      ]),
    )
    expect(stats).toEqual({ cached: 2, total: 3 })
  })

  it('does not double count repeated lines', () => {
    const stats = parseBuildCacheStats(
      mk(['#2 [1/2] FROM x', '#2 CACHED', '#2 CACHED']),
    )
    expect(stats).toEqual({ cached: 1, total: 1 })
  })

  it('returns null without BuildKit steps', () => {
    expect(parseBuildCacheStats(mk(['npm ERR! boom']))).toBeNull()
    expect(parseBuildCacheStats([])).toBeNull()
  })
})
