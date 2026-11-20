import { describe, expect, it } from 'vitest'
import type { ImportAnalysis } from '../../../queries/importAnalyze'
import {
  siteNameFromRepo,
  gitSteps,
  stageFromDeployment,
  staticDetection,
} from './pagesGitLogic'

function analysis(over: Partial<ImportAnalysis>): ImportAnalysis {
  return {
    stack: { runtime: 'node', framework: 'Vite', evidence: '' },
    build: { type: 'static', evidence: '', alternatives: [] },
    detection: {
      framework: 'Vite',
      build_command: 'npm run build',
      output_dir: 'dist',
      spa: true,
    },
    ...over,
  } as unknown as ImportAnalysis
}

describe('staticDetection', () => {
  it('prefills the build command and output directory with the brand logo', () => {
    const d = staticDetection(analysis({}))
    expect(d).toMatchObject({
      isStatic: true,
      buildCommand: 'npm run build',
      outputDir: 'dist',
      spa: true,
      logo: 'vite',
    })
  })

  it('flags a server app and drops the logo for an unknown framework', () => {
    const d = staticDetection(
      analysis({
        stack: { runtime: 'node', framework: 'Zzz', evidence: '' },
        build: { type: 'railpack', evidence: '', alternatives: [] },
        detection: undefined,
      }),
    )
    expect(d.isStatic).toBe(false)
    expect(d.logo).toBeNull()
    expect(d.buildCommand).toBe('')
  })
})

describe('stageFromDeployment', () => {
  it('walks cloning, building, ready and failed', () => {
    expect(stageFromDeployment(null, 0)).toBe('cloning')
    expect(stageFromDeployment('building', 0)).toBe('cloning')
    expect(stageFromDeployment('building', 2)).toBe('building')
    expect(stageFromDeployment('ready', 3)).toBe('ready')
    expect(stageFromDeployment('failed', 1)).toBe('failed')
  })
})

describe('siteNameFromRepo', () => {
  it('keeps the allowed pattern', () => {
    expect(siteNameFromRepo('My_Site.git')).toBe('my-site')
  })
})

describe('gitSteps', () => {
  it('keeps four ordered rows and maps each state', () => {
    const rows = gitSteps(
      [
        { name: 'cloning', state: 'done' },
        { name: 'building', state: 'failed' },
        { name: 'publishing', state: 'unknown' },
      ],
      'failed',
    )
    expect(rows.map((r) => `${r.name}:${r.state}`)).toEqual([
      'cloning:done',
      'building:failed',
      'publishing:unknown',
      'live:pending',
    ])
  })

  it('treats an unrecognised state as unknown and a ready build as done', () => {
    expect(
      gitSteps([{ name: 'cloning', state: 'weird' }], null)[0]?.state,
    ).toBe('unknown')
    expect(gitSteps(undefined, 'ready').every((r) => r.state === 'done')).toBe(
      true,
    )
  })
})
