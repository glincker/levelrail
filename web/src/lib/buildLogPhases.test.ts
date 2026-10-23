import { describe, expect, it } from 'vitest'
import type { LogLine } from '../hooks/useLogStream'
import {
  analyzeBuildLog,
  formatStepDuration,
  hasRecognisedPhases,
} from './buildLogPhases'

function toLines(texts: string[]): LogLine[] {
  return texts.map((line, i) => ({ id: i + 1, line, stream: 'stdout' }))
}

describe('analyzeBuildLog', () => {
  it('classifies npm ci as install with duration', () => {
    const a = analyzeBuildLog(
      toLines([
        '#12 [stage-1 3/7] RUN npm ci',
        '#12 1.2 added 312 packages in 12s',
        '#12 DONE 14.2s',
      ]),
    )
    expect(a.steps).toEqual([
      {
        id: '12',
        label: 'RUN npm ci',
        phase: 'install',
        cached: false,
        durationSec: 14.2,
        firstLine: 0,
        lastLine: 2,
      },
    ])
    expect(a.linePhases).toEqual(['install', 'install', 'install'])
    expect(a.durations.install).toBeCloseTo(14.2)
    expect(hasRecognisedPhases(a)).toBe(true)
  })

  it('classifies pip install as install', () => {
    const a = analyzeBuildLog(
      toLines([
        '#7 [3/6] RUN pip install --no-cache-dir -r requirements.txt',
        '#7 DONE 8.0s',
      ]),
    )
    expect(a.steps[0]?.phase).toBe('install')
  })

  it('classifies go build as build and a RUN that is not an install as build', () => {
    const a = analyzeBuildLog(
      toLines([
        '#9 [build 5/6] RUN go build -o /app ./cmd/server',
        '#9 DONE 20.5s',
        '#10 [build 6/6] RUN echo hello',
        '#10 DONE 0.3s',
      ]),
    )
    expect(a.steps.map((s) => s.phase)).toEqual(['build', 'build'])
    expect(a.durations.build).toBeCloseTo(20.8)
  })

  it('a combined install and build RUN counts as build', () => {
    const a = analyzeBuildLog(
      toLines(['#4 [2/3] RUN npm ci && npm run build', '#4 DONE 30.0s']),
    )
    expect(a.steps[0]?.phase).toBe('build')
  })

  it('classifies docker export and push steps as push', () => {
    const a = analyzeBuildLog(
      toLines([
        '#15 exporting to image',
        '#15 exporting layers 1.1s done',
        '#15 DONE 1.4s',
        '#16 pushing layers 2.0s done',
        '#16 DONE 2.5s',
      ]),
    )
    expect(a.steps.map((s) => s.phase)).toEqual(['push', 'push'])
    expect(a.counts.push).toBe(5)
    expect(a.durations.push).toBeCloseTo(3.9)
  })

  it('marks CACHED steps and excludes them from phase time', () => {
    const a = analyzeBuildLog(
      toLines(['#6 [2/5] RUN npm ci', '#6 CACHED', '#6 DONE 0.0s']),
    )
    expect(a.steps[0]?.cached).toBe(true)
    expect(a.durations.install).toBe(0)
  })

  it('leaves COPY and internal steps as other', () => {
    const a = analyzeBuildLog(
      toLines([
        '#1 [internal] load build definition from Dockerfile',
        '#1 DONE 0.0s',
        '#5 [stage-1 2/7] COPY package.json .',
        '#5 DONE 0.1s',
      ]),
    )
    expect(a.steps.map((s) => s.phase)).toEqual(['other', 'other'])
    expect(hasRecognisedPhases(a)).toBe(false)
  })

  it('does not start steps from timing or sha lines of unknown ids', () => {
    const a = analyzeBuildLog(
      toLines([
        '#3 0.512 hello',
        '#3 sha256:abc 1MB / 2MB 0.5s',
        '#3 DONE 1.0s',
      ]),
    )
    expect(a.steps).toEqual([])
  })

  it('classifies unmarked prompt lines and output signatures', () => {
    const a = analyzeBuildLog(
      toLines([
        '$ pnpm install',
        'Progress: resolved 10, reused 10',
        '$ vite build',
        'Please run npm install to fix this',
      ]),
    )
    expect(a.linePhases).toEqual(['install', 'install', 'build', 'other'])
  })

  it('follows legacy Step markers until the next one', () => {
    const a = analyzeBuildLog(
      toLines([
        'Step 3/7 : RUN yarn install',
        ' ---> Running in abc',
        'some output',
        'Step 4/7 : COPY . .',
        'more',
      ]),
    )
    expect(a.linePhases).toEqual([
      'install',
      'install',
      'install',
      'other',
      'other',
    ])
  })

  it('strips ANSI codes before matching', () => {
    const a = analyzeBuildLog(
      toLines(['\u001b[36m#2 [1/2] RUN apk add curl\u001b[0m']),
    )
    expect(a.steps[0]?.phase).toBe('install')
  })

  it('never throws on malformed input', () => {
    const bad = [
      { id: 1, line: undefined, stream: 'stdout' },
      { id: 2, line: '#', stream: 'stdout' },
      { id: 3, line: '#abc DONE xs', stream: 'stdout' },
      { id: 4, line: '#9 DONE 1e999s', stream: 'stdout' },
      { id: 5, line: '', stream: 'stdout' },
    ] as unknown as LogLine[]
    const a = analyzeBuildLog(bad)
    expect(a.linePhases).toHaveLength(5)
    expect(a.steps).toEqual([])
  })

  it('caps analysis at the line limit and reports truncation', () => {
    const a = analyzeBuildLog(toLines(['$ npm ci', '$ npm ci', '$ npm ci']), 2)
    expect(a.linePhases).toEqual(['install', 'install', 'other'])
    expect(a.truncated).toBe(true)
  })

  it('handles an empty log', () => {
    const a = analyzeBuildLog([])
    expect(a.steps).toEqual([])
    expect(hasRecognisedPhases(a)).toBe(false)
    expect(a.truncated).toBe(false)
  })
})

describe('formatStepDuration', () => {
  it('formats seconds and minutes', () => {
    expect(formatStepDuration(14.24)).toBe('14.2s')
    expect(formatStepDuration(125)).toBe('2m 5s')
    expect(formatStepDuration(-1)).toBe('')
  })
})
