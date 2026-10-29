import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { buildReleaseRows } from '@/lib/appReleases'
import type { AppRelease } from '../../types/appRelease'
import { AppReleasesList } from './AppReleases'

const rel = (id: string, created_at: string): AppRelease => ({
  id,
  image_ref: `app@sha256:${id}`,
  digest: `sha256:${id.repeat(16)}`,
  short_digest: id.repeat(2),
  created_at,
  source_reason: 'deploy',
})

describe('buildReleaseRows', () => {
  it('sorts newest first and marks the current release', () => {
    const rows = buildReleaseRows(
      [rel('a', '2026-01-01T00:00:00Z'), rel('b', '2026-02-01T00:00:00Z')],
      'a',
    )
    expect(rows.map((r) => r.release.id)).toEqual(['b', 'a'])
    expect(rows.map((r) => r.isCurrent)).toEqual([false, true])
  })
})

describe('AppReleasesList', () => {
  it('renders nothing when empty', () => {
    const { container } = render(<AppReleasesList data={{ releases: [] }} />)
    expect(container.firstChild).toBeNull()
  })
  it('renders rows with the current marker', () => {
    render(
      <AppReleasesList
        data={{
          releases: [rel('a', '2026-01-01T00:00:00Z')],
          current_release_id: 'a',
        }}
      />,
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(1)
    expect(screen.getByText('deploy')).toBeTruthy()
  })
})
