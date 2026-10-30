import { render, type RenderResult } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { AllI18n } from './allI18n'
import { createTestQueryClient } from './testQueryClient'

/** Mounts `ui` under a fresh QueryClient only. */
export function renderWithQuery(ui: ReactNode): RenderResult {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      {ui}
    </QueryClientProvider>,
  )
}

/** Same as renderWithQuery with every real en catalogue loaded. */
export function renderWithQueryI18n(ui: ReactNode): RenderResult {
  return render(
    <AllI18n>
      <QueryClientProvider client={createTestQueryClient()}>
        {ui}
      </QueryClientProvider>
    </AllI18n>,
  )
}
