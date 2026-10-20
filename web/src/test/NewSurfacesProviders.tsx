import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { newSurfacesI18n } from './newSurfacesI18n'

/** Renders children with the real common, databases, functions, images and storageBuckets catalogues and a fresh query client. */
export function NewSurfacesProviders({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={newSurfacesI18n}>{children}</I18nextProvider>
    </QueryClientProvider>
  )
}
