import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SiteCiCard from './SiteCiCard'
import { ciSnippets } from './ciSnippets'

const h = vi.hoisted(() => ({ create: vi.fn() }))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))
vi.mock('../../queries/pagesCiTokens', () => ({
  useSiteDeployTokens: () => ({
    isSuccess: true,
    isPending: false,
    isError: false,
    data: [],
  }),
  useCreateSiteDeployToken: () => ({
    mutate: h.create,
    isPending: false,
    error: null,
  }),
  useRevokeSiteDeployToken: () => ({
    mutate: vi.fn(),
    isPending: false,
    error: null,
  }),
}))

beforeEach(() => h.create.mockReset())

describe('SiteCiCard', () => {
  it('creates a token on click', () => {
    render(<SiteCiCard name="docs" />)
    fireEvent.click(screen.getByText('ci.create'))
    expect(h.create).toHaveBeenCalledTimes(1)
  })

  it('snippets name the site and never embed a secret', () => {
    const s = ciSnippets('docs', 'https://lr.example.com')
    expect(s.github).toContain('site: docs')
    expect(s.curl).toContain('/api/v1/apps/docs/uploads')
    expect(s.gitlab).toContain('$LEVELRAIL_PAGES_TOKEN')
  })
})
