import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ImagesBanner } from '../images/ImagesBanner'
import { ZonesBanner } from '../dns/ZonesBanner'

vi.mock('@/components/HelpLink', () => ({
  HelpLink: ({ label }: { label: string }) => <a href="/help">{label}</a>,
}))

describe('pass 3 page banners', () => {
  it('shows the images hero when the node has no images and compact otherwise', () => {
    const { rerender } = render(
      <ImagesBanner empty action={<button type="button">clean</button>} />,
    )
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(screen.getByRole('heading', { level: 1 })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'clean' })).toBeTruthy()

    rerender(
      <ImagesBanner
        empty={false}
        action={<button type="button">clean</button>}
      />,
    )
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.queryByTestId('page-banner-full')).toBeNull()
  })

  it('shows the DNS hero before any zone exists and compact once zones are listed', () => {
    const { rerender } = render(<ZonesBanner empty helpLabel="DNS guide" />)
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(screen.getByText('DNS guide')).toBeTruthy()

    rerender(<ZonesBanner empty={false} helpLabel="DNS guide" />)
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.queryByTestId('page-banner-full')).toBeNull()
  })
})
