import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  RouterProvider,
  createMemoryHistory,
  createRootRoute,
  createRouter,
} from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { AllI18n } from './allI18n'

/** Mounts `ui` as the root route so Links, useRouter and i18n all work; no network. */
export async function renderInRouter(ui: ReactNode, queryClient?: QueryClient) {
  const qc =
    queryClient ??
    new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const root = createRootRoute({ component: () => <>{ui}</> })
  const router = createRouter({
    routeTree: root,
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  await router.load()
  return render(
    <AllI18n>
      <QueryClientProvider client={qc}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </AllI18n>,
  )
}
