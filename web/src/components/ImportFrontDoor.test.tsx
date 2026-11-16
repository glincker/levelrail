import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it } from 'vitest'
import { AllI18n } from '../test/allI18n'
import { createTestQueryClient } from '@/test/testQueryClient'
import { ImportFrontDoor } from './ImportFrontDoor'

describe('ImportFrontDoor', () => {
  it('shows the submit shortcut hint and links it to the field', () => {
    render(
      <AllI18n>
        <QueryClientProvider client={createTestQueryClient()}>
          <ImportFrontDoor onPlan={() => {}} />
        </QueryClientProvider>
      </AllI18n>,
    )
    expect(screen.getByText(/Press (Ctrl|Cmd)\+Enter to continue/)).toBeTruthy()
    expect(screen.getByRole('textbox').getAttribute('aria-describedby')).toBe(
      'import-front-door-hint',
    )
  })
})
