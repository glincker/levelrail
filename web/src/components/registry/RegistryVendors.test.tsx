import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import settingsEn from '../../locales/en/settings.json'
import { RegistryVendorGrid } from './RegistryVendorGrid'
import { PackageRegistriesCard } from './PackageRegistriesCard'
import type { RegistryVendor } from '../../types/registryVendor'

const vendors: RegistryVendor[] = [
  {
    id: 'ghcr',
    name: 'GitHub Container Registry',
    default_host: 'ghcr.io',
    brand: 'github',
  },
  {
    id: 'ecr',
    name: 'AWS ECR',
    host_hint: '<account>.dkr.ecr.<region>.amazonaws.com',
    fixed_username: 'AWS',
    token_lifetime_hours: 12,
    brand: 'ecr',
  },
]

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const router = vi.hoisted(() => ({
  search: {},
  listeners: new Set<() => void>(),
}))

vi.mock('@tanstack/react-router', async () => {
  const React = await import('react')
  return {
    useSearch: () =>
      React.useSyncExternalStore(
        (cb) => {
          router.listeners.add(cb)
          return () => router.listeners.delete(cb)
        },
        () => router.search,
      ),
    useNavigate: () => (opts: { search: Record<string, unknown> }) => {
      router.search = opts.search
      router.listeners.forEach((l) => l())
      return Promise.resolve()
    },
  }
})

const createMutate = vi.fn()
const probeMutate = vi.fn()
const createPackageMutate = vi.fn()
const idle = { isPending: false, isError: false, mutate: vi.fn() }

vi.mock('../../queries/registryCredentials', () => ({
  useCreateRegistryCredential: () => ({ ...idle, mutate: createMutate }),
}))
vi.mock('../../queries/registryVendors', () => ({
  useRegistryVendors: () => ({
    data: vendors,
    isLoading: false,
    isError: false,
  }),
  useProbeRegistryCredential: () => ({ ...idle, mutate: probeMutate }),
  usePackageRegistries: () => ({ data: [], isError: false }),
  useCreatePackageRegistry: () => ({ ...idle, mutate: createPackageMutate }),
  useDeletePackageRegistry: () => idle,
  useTestPackageRegistry: () => idle,
}))

function renderWith(node: React.ReactNode) {
  return render(<I18nextProvider i18n={testI18n}>{node}</I18nextProvider>)
}

describe('registry vendor presets', () => {
  beforeEach(() => {
    router.search = {}
  })
  afterEach(() => {
    cleanup()
    createMutate.mockReset()
    probeMutate.mockReset()
    createPackageMutate.mockReset()
  })

  it('opens a vendor from a deep link with host and scopes shown', () => {
    router.search = { connect: 'ghcr' }
    renderWith(<RegistryVendorGrid />)
    expect(screen.getByLabelText('Registry host')).toHaveValue('ghcr.io')
    expect(screen.getByText(/read:packages to pull/)).toBeVisible()
  })

  it('fixes the ECR username and records a short lived expiry', () => {
    router.search = { connect: 'ecr' }
    renderWith(<RegistryVendorGrid />)
    expect(screen.getByLabelText('Username')).toHaveValue('AWS')
    expect(screen.getByLabelText('Expires (optional)')).not.toHaveValue('')
  })

  it('tests the candidate before saving and never saves on test', () => {
    router.search = { connect: 'ghcr' }
    renderWith(<RegistryVendorGrid />)
    fireEvent.change(screen.getByLabelText('Username'), {
      target: { value: 'octo' },
    })
    fireEvent.change(screen.getByLabelText('Password or token'), {
      target: { value: 'pat-value' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Test connection' }))
    expect(probeMutate).toHaveBeenCalledWith(
      { registry_host: 'ghcr.io', username: 'octo', password: 'pat-value' },
      expect.anything(),
    )
    expect(createMutate).not.toHaveBeenCalled()
  })

  it('opens the package registry dialog and submits an npm registry', () => {
    router.search = { connect: 'package-registry', ecosystem: 'npm' }
    renderWith(<PackageRegistriesCard />)
    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'corp' },
    })
    fireEvent.change(screen.getByLabelText('Registry URL'), {
      target: { value: 'https://npm.example.com' },
    })
    fireEvent.change(screen.getByLabelText('Token or password'), {
      target: { value: 'tok' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save registry' }))
    expect(createPackageMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'corp',
        ecosystem: 'npm',
        url: 'https://npm.example.com',
        secret: 'tok',
        scope_kind: 'instance',
      }),
      expect.anything(),
    )
  })
})
