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
  it('renders the hero size with heading, action and illustration', () => {
    renderBanner('hero')
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(screen.getByRole('heading', { level: 1 })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'act' })).toBeTruthy()
    expect(screen.getByTestId('art')).toBeTruthy()
  })

  it('renders the compact size as one row that keeps heading, action and art', () => {
    renderBanner('compact')
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.queryByTestId('page-banner-full')).toBeNull()
    expect(screen.getByRole('button', { name: 'act' })).toBeTruthy()
    expect(screen.getByTestId('art')).toBeTruthy()
  })
})
