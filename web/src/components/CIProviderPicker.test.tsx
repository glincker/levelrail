import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { CIProviderPicker } from './CIProviderPicker'
import { staticSiteSnippet } from '../lib/ciStaticSnippets'

const OPTIONS = [
  { id: 'github', label: 'GitHub Actions' },
  { id: 'gitlab', label: 'GitLab CI' },
  { id: 'circleci', label: 'CircleCI' },
  { id: 'jenkins', label: 'Jenkins' },
  { id: 'buildkite', label: 'Buildkite' },
]

function Harness() {
  const [value, setValue] = useState('github')
  return (
    <CIProviderPicker
      label="CI provider"
      options={OPTIONS}
      value={value}
      onChange={setValue}
    />
  )
}

describe('CIProviderPicker', () => {
  it('renders one radio per provider and marks the selected one', () => {
    render(<Harness />)
    expect(screen.getAllByRole('radio')).toHaveLength(5)
    expect(screen.getByRole('radio', { name: 'GitHub Actions' })).toBeChecked()
  })

  it('selects with the arrow keys and wraps around', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByRole('radio', { name: 'GitHub Actions' }))
    await user.keyboard('{ArrowRight}')
    expect(screen.getByRole('radio', { name: 'GitLab CI' })).toBeChecked()
    await user.keyboard('{ArrowLeft}{ArrowLeft}')
    expect(screen.getByRole('radio', { name: 'Buildkite' })).toBeChecked()
    await user.keyboard('{Home}')
    expect(screen.getByRole('radio', { name: 'GitHub Actions' })).toBeChecked()
  })
})

describe('staticSiteSnippet', () => {
  it.each(OPTIONS.map((o) => o.id))(
    'publishes the folder for %s without building an image',
    (id) => {
      const snip = staticSiteSnippet(id, 'docs')
      expect(snip?.content).toContain('pages deploy ./dist --name docs')
      expect(snip?.content).toContain('LEVELRAIL_TOKEN')
      expect(snip?.content).not.toContain('docker build')
    },
  )

  it('returns nothing for an unknown provider', () => {
    expect(staticSiteSnippet('nope', 'docs')).toBeUndefined()
  })
})
