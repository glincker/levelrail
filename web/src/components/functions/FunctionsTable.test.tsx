import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { NewSurfacesProviders } from '../../test/NewSurfacesProviders'
import { newSurfacesI18n } from '../../test/newSurfacesI18n'
import { FunctionsTable } from './FunctionsTable'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      params,
    }: {
      children: ReactNode
      params: { name: string }
    }) => <a href={`/apps/${params.name}`}>{children}</a>,
  }
})

const base = {
  image: 'img',
  port: 8080,
  idle_minutes: 5,
  invocations: 1200,
  error_rate_5xx: 0,
  p95_ms: 250,
}

describe('FunctionsTable', () => {
  it('shows each function with its sleep state and links to the app', () => {
    render(
      <NewSurfacesProviders>
        <FunctionsTable
          items={[
            { ...base, name: 'hello', sleeping: true },
            { ...base, name: 'worker', sleeping: false },
          ]}
        />
      </NewSurfacesProviders>,
    )
    expect(screen.getByRole('link', { name: 'hello' })).toHaveAttribute(
      'href',
      '/apps/hello',
    )
    expect(
      screen.getByText(newSurfacesI18n.t('list.asleep', { ns: 'functions' })),
    ).toBeInTheDocument()
    expect(
      screen.getByText(newSurfacesI18n.t('list.awake', { ns: 'functions' })),
    ).toBeInTheDocument()
  })
})
