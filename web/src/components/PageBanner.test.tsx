import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { PageBanner } from './PageBanner'

function renderBanner(compact: boolean) {
  render(
    <PageBanner
      compact={compact}
      icon={<svg data-testid="icon" />}
      titleKey="banner.title"
      descriptionKey="banner.description"
      action={<button type="button">act</button>}
      illustration={<svg data-testid="art" />}
    />,
  )
}

describe('PageBanner', () => {
  it('renders heading, action and illustration when full', () => {
    renderBanner(false)
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(screen.getByRole('heading', { level: 1 })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'act' })).toBeTruthy()
    expect(screen.getByTestId('art')).toBeTruthy()
  })

  it('keeps heading and action but drops the illustration when compact', () => {
    renderBanner(true)
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.queryByTestId('page-banner-full')).toBeNull()
    expect(screen.getByRole('button', { name: 'act' })).toBeTruthy()
    expect(screen.queryByTestId('art')).toBeNull()
  })
})
