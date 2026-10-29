import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PinnedDigestBadge } from './PinnedDigestBadge'
import { digestOfImage, shortDigest } from '../lib/imageDigest'

const REF = `app:v1@sha256:${'ab12'.repeat(16)}`

describe('digest helpers', () => {
  it('extracts and shortens the digest', () => {
    expect(digestOfImage(REF)).toBe(`sha256:${'ab12'.repeat(16)}`)
    expect(shortDigest(digestOfImage(REF))).toBe('ab12ab12ab12')
    expect(digestOfImage('app:v1')).toBe('')
  })
})

describe('PinnedDigestBadge', () => {
  it('shows the short digest with a copy button', () => {
    render(<PinnedDigestBadge digest={digestOfImage(REF)} />)
    expect(screen.getByText('ab12ab12ab12')).toBeTruthy()
    expect(screen.getByRole('button')).toBeTruthy()
  })
  it('renders nothing without a digest', () => {
    const { container } = render(<PinnedDigestBadge digest="" />)
    expect(container.firstChild).toBeNull()
  })
})
