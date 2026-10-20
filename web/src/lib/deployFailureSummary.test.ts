import { describe, expect, it } from 'vitest'
import type { LogLine } from '../hooks/useLogStream'
import type { DeployAttempt } from '../types/deployAttempt'
import { matchHintForText } from './buildLogHints'
import {
  findLastGoodAttempt,
  summarizeDeployFailure,
} from './deployFailureSummary'
import type { DeployStage } from './deployStages'

describe('matchHintForText', () => {
  const cases: Array<[string, string, string | undefined]> = [
    [
      'missing env',
      'Error: environment variable DATABASE_URL is required',
      'missing-env-var',
    ],
    ['python keyerror', "KeyError: 'SECRET_KEY'", 'missing-env-var'],
    [
      'port in use',
      'Error: listen EADDRINUSE: address already in use :::3000',
      'port-mismatch',
    ],
    ['oom', 'container exited: OOMKilled', 'oom-killed'],
    ['exit 137', 'process exited with code 137', 'oom-killed'],
    [
      'registry auth',
      'pull access denied for acme/web, repository does not exist or may require authorization',
      'registry-auth',
    ],
    ['image missing', 'manifest unknown: manifest unknown', 'image-not-found'],
    [
      'base image',
      'failed to resolve source metadata for docker.io/library/nod:20: docker.io/library/nod:20: not found',
      'image-not-found',
    ],
    [
      'readiness',
      'ReadinessFailed: container did not become ready within 60s',
      'health-check-timeout',
    ],
    [
      'dockerfile',
      'failed to solve with frontend dockerfile.v0: failed to read dockerfile: open Dockerfile: no such file or directory',
      'dockerfile-path',
    ],
    ['npm resolve', 'npm ERR! code ERESOLVE', 'dependency-install'],
    [
      'pip',
      'ERROR: No matching distribution found for foo==9.9',
      'dependency-install',
    ],
    [
      'go mod',
      'go: example.com/x@v1.0.0: unrecognized import path',
      'dependency-install',
    ],
    ['unrelated', 'Compiled successfully in 3.2s', undefined],
  ]
  it.each(cases)('%s', (_name, text, id) => {
    expect(matchHintForText(text)?.id).toBe(id)
  })
})

const attempt = (over: Partial<DeployAttempt>): DeployAttempt => ({
  id: 'a1',
  service_name: 'web',
  image: 'web:1',
  status: 'failed',
  started_at: '2026-09-24T10:00:00Z',
  ...over,
})

const failedBuild = (detail?: string): DeployStage[] => [
  { key: 'build', label: 'Build', status: 'failed', detail },
  { key: 'rollout', label: 'Roll out', status: 'skipped' },
]

const line = (id: number, text: string): LogLine => ({
  id,
  line: text,
  stream: 'stderr',
})

describe('summarizeDeployFailure', () => {
  it('returns null when no stage failed', () => {
    const stages: DeployStage[] = [
      { key: 'build', label: 'Build', status: 'done' },
    ]
    expect(
      summarizeDeployFailure({
        stages,
        conditions: [],
        lines: [],
        attempt: attempt({}),
      }),
    ).toBeNull()
  })

  it('prefers the error message for the cause', () => {
    const s = summarizeDeployFailure({
      stages: failedBuild(
        'failed to read dockerfile: open Dockerfile: no such file',
      ),
      conditions: [],
      lines: [line(1, 'npm ERR! code ERESOLVE')],
      attempt: attempt({}),
    })
    expect(s?.stageLabel).toBe('Build')
    expect(s?.cause?.id).toBe('dockerfile-path')
    expect(s?.evidence).toBeUndefined()
  })

  it('falls back to the newest matching log line with evidence', () => {
    const s = summarizeDeployFailure({
      stages: failedBuild('exit status 1'),
      conditions: [],
      lines: [
        line(1, 'npm ERR! code ETIMEDOUT'),
        line(
          2,
          '\u001b[31mFATAL ERROR: JavaScript heap out of memory\u001b[0m',
        ),
      ],
      attempt: attempt({}),
    })
    expect(s?.cause?.id).toBe('node-heap-oom')
    expect(s?.evidence).toBe('FATAL ERROR: JavaScript heap out of memory')
  })

  it('uses failing conditions for a roll out failure', () => {
    const s = summarizeDeployFailure({
      stages: [
        { key: 'build', label: 'Build', status: 'done' },
        {
          key: 'rollout',
          label: 'Roll out',
          status: 'failed',
          detail: 'container exited',
        },
      ],
      conditions: [
        {
          Type: 'Ready',
          Status: 'False',
          Reason: 'OOMKilledDuringReadiness',
          Message: 'container was killed',
          LastTransitionTime: '2026-09-24T10:01:00Z',
        },
      ],
      lines: [],
      attempt: attempt({ status: 'succeeded' }),
    })
    expect(s?.stageLabel).toBe('Roll out')
    expect(s?.cause?.id).toBe('oom-killed')
  })

  it('reports a placeholder when nothing was recorded', () => {
    const s = summarizeDeployFailure({
      stages: failedBuild(undefined),
      conditions: [],
      lines: [],
      attempt: attempt({}),
    })
    expect(s?.message).toMatch(/No error message/)
    expect(s?.cause).toBeUndefined()
  })
})

describe('findLastGoodAttempt', () => {
  const list = [
    attempt({ id: 'c', started_at: '2026-09-24T12:00:00Z' }),
    attempt({
      id: 'b',
      status: 'succeeded',
      started_at: '2026-09-24T11:00:00Z',
    }),
    attempt({
      id: 'a',
      status: 'succeeded',
      started_at: '2026-09-24T09:00:00Z',
    }),
  ]
  it('picks the newest earlier success', () => {
    expect(findLastGoodAttempt(list, list[0]!)?.id).toBe('b')
  })
  it('returns undefined when no earlier success exists', () => {
    expect(findLastGoodAttempt(list, list[2]!)).toBeUndefined()
  })
})
