import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PageBanner, type PageBannerSize } from './PageBanner'

function renderBanner(size: PageBannerSize) {
  render(
    <PageBanner
      size={size}
      icon={<svg data-testid="icon" />}
      titleKey="banner.title"
      descriptionKey="banner.description"
      action={<button type="button">act</button>}
      illustration={<svg data-testid="art" />}
    />,
  )
}

describe('PageBanner', () => {
  it('renders the hero size with heading and action, without the retired illustration', () => {
    renderBanner('hero')
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(screen.getByRole('heading', { level: 1 })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'act' })).toBeTruthy()
    expect(screen.queryByTestId('art')).toBeNull()
  })

  it('renders the compact size as one row that keeps heading and action', () => {
    renderBanner('compact')
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.queryByTestId('page-banner-full')).toBeNull()
    expect(screen.getByRole('button', { name: 'act' })).toBeTruthy()
    expect(screen.queryByTestId('art')).toBeNull()
  })

  it('draws the icon art only at hero size', () => {
    const { unmount } = render(
      <PageBanner
        icon={<svg data-testid="icon" />}
        titleKey="banner.title"
        descriptionKey="banner.description"
      />,
    )
    expect(screen.queryByTestId('icon')).not.toBeNull()
    unmount()
    renderBanner('compact')
    expect(screen.queryByTestId('icon')).toBeNull()
  })
  it('keeps the help control in the action row, never absolutely positioned', () => {
    render(
      <PageBanner
        icon={<svg />}
        titleKey="banner.title"
        descriptionKey="banner.description"
        action={
          <>
            <button type="button">primary</button>
            <a data-slot="help-link" href="/help" aria-label="Help">
              ?
            </a>
          </>
        }
      />,
    )
    const header = screen.getByTestId('page-banner-full')
    const help = screen.getByLabelText('Help')
    const primary = screen.getByRole('button', { name: 'primary' })
    expect(help.parentElement).toBe(primary.parentElement)
    expect(help.parentElement?.className).toContain('flex')
    expect(header.className).not.toContain('help-link')
    expect(header.className).not.toMatch(/\babsolute\b/)
  })
})
