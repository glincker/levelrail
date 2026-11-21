import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import { AppWiring } from './AppWiring'
import { linkedDatabases, registryHost } from './wiringModel'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (k: string, o?: Record<string, unknown>) =>
      o && 'count' in o ? `${k}:${String(o.count)}` : k,
  }),
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: React.ReactNode }) => <a>{children}</a>,
}))
vi.mock('@/components/kit', () => ({
  StatusPill: ({ label }: { label: string }) => <span>{label}</span>,
}))
vi.mock('../../queries/certificates', () => ({
  useCertificates: () => ({
    data: [{ domain: 'a.example.com', status: 'healthy' }],
  }),
}))
vi.mock('../../queries/gitSources', () => ({
  useGitSource: () => ({ data: undefined }),
}))

function app(over: Partial<AppDetail>): AppDetail {
  return {
    name: 'web',
    image: 'nginx:1',
    port: 80,
    bind_address: 'private',
    suspended: false,
    env_dirty: false,
    strategy: 'rolling',
    replicas: 2,
    ...over,
  }
}

describe('AppWiring', () => {
  it('shows an action per empty group', () => {
    render(<AppWiring app={app({})} conditions={[]} />)
    expect(screen.getByText('wiring.noDomains')).toBeInTheDocument()
    expect(screen.getByText('wiring.noStorage')).toBeInTheDocument()
    expect(screen.getByText('wiring.noEnv')).toBeInTheDocument()
    expect(screen.getByText('wiring.noNode')).toBeInTheDocument()
  })

  it('counts domains, storage and environment', () => {
    render(
      <AppWiring
        app={app({
          domains: ['a.example.com'],
          volumes: [{ name: 'data', container_path: '/d' }],
          database_attachment: {
            database_name: 'pg',
            env_var: 'DATABASE_URL',
            field: 'url',
          },
          env: { A: '1', B: '2' },
          secret_env: ['S'],
        })}
        conditions={[]}
      />,
    )
    expect(screen.getByText('a.example.com')).toBeInTheDocument()
    expect(screen.getByText('wiring.tls.healthy')).toBeInTheDocument()
    expect(screen.getByText('wiring.envSummary')).toBeInTheDocument()
    expect(screen.getAllByText('pg').length).toBeGreaterThan(0)
  })

  it('derives linked databases and registry host', () => {
    expect(
      linkedDatabases(
        app({
          database_env: {
            X: { database: 'pg', field: 'url' },
            Y: { database: 'pg', field: 'host' },
          },
        }),
      ),
    ).toEqual(['pg'])
    expect(registryHost('ghcr.io/o/r:1')).toBe('ghcr.io')
    expect(registryHost('nginx:1')).toBeNull()
    expect(registryHost('library/nginx')).toBeNull()
  })
})
