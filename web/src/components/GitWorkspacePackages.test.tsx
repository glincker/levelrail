import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import gitBuildEn from '../locales/en/gitBuild.json'
import { GitWorkspacePackages } from './GitWorkspacePackages'
import type { WorkspacePackage } from '../types/gitSource'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['gitBuild'],
  defaultNS: 'gitBuild',
  resources: { en: { gitBuild: gitBuildEn } },
  interpolation: { escapeValue: false },
})

const web: WorkspacePackage = {
  name: 'web',
  dir: 'apps/web',
  tool: 'pnpm-workspace',
  depends_on: ['packages/ui'],
  watch_paths: ['apps/web/**', 'packages/ui/**', 'pnpm-lock.yaml'],
}

function renderPanel(onUse = vi.fn(), packages = [web]) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <GitWorkspacePackages
          appName="web"
          packages={packages}
          applying={false}
          onUse={onUse}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
  return onUse
}

describe('GitWorkspacePackages', () => {
  it('renders nothing without packages', () => {
    renderPanel(vi.fn(), [])
    expect(screen.queryByText('Workspace packages')).toBeNull()
  })

  it('lists watch paths and applies a package', async () => {
    const onUse = renderPanel()
    expect(screen.getByText('apps/web')).toBeTruthy()
    expect(screen.getByText(/packages\/ui\/\*\*/)).toBeTruthy()
    await userEvent.click(
      screen.getByRole('button', { name: 'Use this package' }),
    )
    expect(onUse).toHaveBeenCalledWith(web)
  })
})
