import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { EnvironmentSwitcher } from './EnvironmentSwitcher'
import { env, renderWithProviders } from './environmentsTestUtils'
import {
  getEnvironmentScope,
  resetEnvironmentScopeForTests,
} from '../../lib/environmentScope'

const features = vi.hoisted(() => ({ list: [] as string[] }))
vi.mock('../../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => features.list,
}))

beforeEach(() => {
  resetEnvironmentScopeForTests()
  window.history.replaceState(null, '', '/')
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify([
            env({ id: 'env_dev', name: 'Development', kind: 'dev' }),
            env({ id: 'preview-env-web', name: 'Preview', kind: 'preview' }),
          ]),
          { status: 200 },
        ),
      ),
    ),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  features.list = []
})

describe('EnvironmentSwitcher', () => {
  it('renders nothing and leaves the scope off while the feature is off', async () => {
    features.list = []
    renderWithProviders(<EnvironmentSwitcher />)
    await new Promise((r) => setTimeout(r, 30))
    expect(
      screen.queryByLabelText(/Filter the dashboard/),
    ).not.toBeInTheDocument()
    expect(getEnvironmentScope()).toBe('')
  })

  it('shows the switcher when the feature is on and restores the stored selection', async () => {
    features.list = ['global-environments']
    window.localStorage.setItem('environment.scope.v1', 'env_dev')
    renderWithProviders(<EnvironmentSwitcher />)
    const trigger = await screen.findByLabelText(/Filter the dashboard/)
    await waitFor(() => {
      expect(trigger).toHaveTextContent('Development')
    })
    expect(getEnvironmentScope()).toBe('env_dev')
  })
})
