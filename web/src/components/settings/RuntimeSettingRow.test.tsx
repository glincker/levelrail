import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import type { RuntimeSetting } from '../../types/runtimeSettings'
import { RuntimeSettingRow } from './RuntimeSettingRow'

const buildNetwork: RuntimeSetting = {
  key: 'security.build_network',
  category: 'security',
  kind: 'string',
  title: 'Build network',
  description: 'Network for RUN steps',
  default: 'open',
  value: 'open',
  source: 'default',
  env: 'APP_BUILD_NETWORK',
  apply: 'live',
  risk: 'weakens_security',
  onboarding: false,
  allowed_strings: ['open', 'none', 'block-private'],
  pending_restart: false,
}

describe('RuntimeSettingRow string kind', () => {
  it('renders a select of the allowed words, not a text area', () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <AllI18n>
          <RuntimeSettingRow setting={buildNetwork} canEdit />
        </AllI18n>
      </QueryClientProvider>,
    )
    expect(screen.queryByRole('textbox')).toBeNull()
    fireEvent.click(screen.getByRole('combobox', { name: 'Build network' }))
    expect(screen.getAllByRole('option')).toHaveLength(3)
    expect(screen.getByRole('option', { name: /^None/ })).toBeTruthy()
  })
})
