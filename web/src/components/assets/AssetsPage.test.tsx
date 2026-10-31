import type { ReactNode } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import assetsEn from '../../locales/en/assets.json'
import commonEn from '../../locales/en/common.json'
import { BucketDetail } from './BucketDetail'
import { BucketList } from './BucketList'
import type { AssetBucket } from '../../types/assets'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'assets'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, assets: assetsEn } },
  interpolation: { escapeValue: false },
})

function wrap(node: ReactNode) {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
    </QueryClientProvider>,
  )
}

const bucket: AssetBucket = {
  id: 'b1',
  name: 'media',
  target_id: 't1',
  preset: 'r2',
  prefix: '',
  visibility: 'private',
  serving_mode: 'direct',
  referrer_allow: [],
  refuse_no_referer: false,
  quota_bytes: 0,
  created_at: '2026-10-25T00:00:00Z',
  updated_at: '2026-10-25T00:00:00Z',
}

const usage = {
  bytes: 2048,
  objects: 7,
  at_least: true,
  quota_bytes: 0,
  serving_mode: 'direct',
  traffic_measured: false,
  cost: {
    estimated: true,
    price_table_date: '2026-10-25',
    stored_bytes: 2048,
    egress_gb: 0,
    current: {
      preset: 'r2',
      storage_usd: 0,
      egress_usd: 0,
      total_usd: 0,
      free_egress_gb: 0,
    },
    compare: [
      {
        preset: 'aws',
        storage_usd: 0.01,
        egress_usd: 0,
        total_usd: 0.01,
        free_egress_gb: 0,
      },
      {
        preset: 'r2',
        storage_usd: 0,
        egress_usd: 0,
        total_usd: 0,
        free_egress_gb: 0,
      },
    ],
    override_envs: ['APP_STORAGE_PRICE_PER_GB_MONTH_R2'],
    notes: [],
  },
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function stub(routes: Record<string, unknown>) {
  const calls: { url: string; init?: RequestInit }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, init })
      const hit = Object.keys(routes).find((k) => url.includes(k))
      return Promise.resolve(hit ? json(routes[hit]) : json({}, 404))
    }),
  )
  return calls
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('BucketList', () => {
  it('shows name, visibility, serving mode and an at-least size', async () => {
    stub({ '/usage': usage })
    wrap(<BucketList buckets={[bucket]} onOpen={vi.fn()} />)
    expect(screen.getByText('media')).toBeInTheDocument()
    expect(screen.getByText('Private')).toBeInTheDocument()
    expect(screen.getByText('Direct')).toBeInTheDocument()
    expect(await screen.findByText('at least 2.0 KiB')).toBeInTheDocument()
  })
})

describe('BucketDetail', () => {
  const base = {
    '/api/v1/assets/buckets/b1/assets': { assets: [] },
    '/usage': usage,
    '/api/v1/assets/buckets/b1': bucket,
  }

  it('offers Fix CORS from a warning and posts fix_cors', async () => {
    const calls = stub({
      '/probe': {
        ok: true,
        steps: [{ name: 'cors', ok: false }],
        cors: { status: 'missing', origin: 'https://x', rule: {} },
        warnings: [
          { code: 'cors_missing', message: 'No CORS rule', fix: 'fix_cors' },
        ],
      },
      ...base,
    })
    wrap(<BucketDetail id="b1" onBack={vi.fn()} />)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Run check' }),
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Fix CORS' }),
    )
    await waitFor(() => {
      const posts = calls.filter((c) => c.url.endsWith('/probe'))
      expect(posts).toHaveLength(2)
      expect(posts[1]?.init?.body).toBe(JSON.stringify({ fix_cors: true }))
    })
  })

  it('shows the exact rule JSON when CORS needs a manual fix', async () => {
    stub({
      '/probe': {
        ok: true,
        steps: [],
        cors: {
          status: 'manual',
          origin: 'https://x',
          rule: { allowed_origins: ['https://x'], allowed_methods: ['PUT'] },
        },
        warnings: [],
      },
      ...base,
    })
    wrap(<BucketDetail id="b1" onBack={vi.fn()} />)
    await userEvent.click(
      await screen.findByRole('button', { name: 'Run check' }),
    )
    expect(
      await screen.findByText('Add this CORS rule at your provider'),
    ).toBeInTheDocument()
    expect(screen.getByText(/allowed_origins/)).toBeInTheDocument()
  })

  it('needs the bucket name typed before making it public', async () => {
    const calls = stub(base)
    wrap(<BucketDetail id="b1" onBack={vi.fn()} />)
    await userEvent.click(await screen.findByRole('switch'))
    const confirm = await screen.findByRole('button', { name: 'Make public' })
    expect(confirm).toBeDisabled()
    await userEvent.type(
      screen.getByRole('textbox', { name: /media/ }),
      'media',
    )
    expect(confirm).toBeEnabled()
    await userEvent.click(confirm)
    await waitFor(() => {
      const patch = calls.find((c) => c.init?.method === 'PATCH')
      expect(patch?.init?.body).toBe(
        JSON.stringify({ visibility: 'public', confirm_name: 'media' }),
      )
    })
  })

  it('labels cost as estimated and compares S3 with R2', async () => {
    stub(base)
    wrap(<BucketDetail id="b1" onBack={vi.fn()} />)
    expect(await screen.findByText('Estimated')).toBeInTheDocument()
    expect(await screen.findByText('AWS S3')).toBeInTheDocument()
    expect(screen.getAllByText(/Cloudflare R2/).length).toBeGreaterThan(0)
    expect(screen.getByLabelText('Egress per month (GB)')).toBeInTheDocument()
  })

  it('has a keyboard reachable choose-files button', async () => {
    stub(base)
    wrap(<BucketDetail id="b1" onBack={vi.fn()} />)
    expect(
      await screen.findByRole('button', { name: 'Choose files' }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('group', { name: 'File drop area' }),
    ).toBeInTheDocument()
  })
})
