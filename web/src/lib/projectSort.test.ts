import { describe, expect, it } from 'vitest'
import type { ProjectResource } from '../types/projectDetail'
import { sortProjects } from './projectSort'

const p = (id: string, name: string, created_at: string) =>
  ({ id, name, created_at }) as ProjectResource

const list = [
  p('1', 'beta', '2026-01-02T00:00:00Z'),
  p('2', 'alpha', '2026-01-03T00:00:00Z'),
  p('3', 'gamma', '2026-01-01T00:00:00Z'),
]

const ids = (xs: ProjectResource[]) => xs.map((x) => x.id)

describe('sortProjects', () => {
  it('sorts by name and by date', () => {
    expect(ids(sortProjects(list, 'name', []))).toEqual(['2', '1', '3'])
    expect(ids(sortProjects(list, 'newest', []))).toEqual(['2', '1', '3'])
    expect(ids(sortProjects(list, 'oldest', []))).toEqual(['3', '1', '2'])
  })
  it('keeps pinned projects first', () => {
    expect(ids(sortProjects(list, 'name', ['3']))).toEqual(['3', '2', '1'])
  })
})
