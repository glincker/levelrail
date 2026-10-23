import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient } from '@tanstack/react-query'
import i18next from 'i18next'
import { isRedirect } from '@tanstack/react-router'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import en from '../../locales/en/canvas.json'
import { devModeQueryOptions } from '../../queries/devMode'
import { requireExperimental } from '../../lib/experimental'
import { IacIssuesError, type IacPlan } from '../../queries/iac'
import { ApiError } from '../../lib/apiError'
import { CanvasLink } from './CanvasLink'
import {
  CanvasReviewDialog,
  type CanvasReviewDialogProps,
} from './CanvasReviewDialog'

vi.mock('@tanstack/react-router', async (orig) => ({
  ...(await orig<typeof import('@tanstack/react-router')>()),
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

let experimental: string[] = []
vi.mock('../../hooks/useExperimental', () => ({
  useExperimentalFeatures: () => experimental,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['canvas'],
  defaultNS: 'canvas',
  resources: { en: { canvas: en } },
  interpolation: { escapeValue: false },
})

function wrap(node: React.ReactNode) {
  return <I18nextProvider i18n={i18n}>{node}</I18nextProvider>
}

describe('Canvas toggle gating', () => {
  it('renders no link while the beta is off', () => {
    experimental = ['iac']
    const { container } = render(wrap(<CanvasLink projectId="p1" />))
    expect(container.innerHTML).toBe('')
  })

  it('renders the link when the beta is on', () => {
    experimental = ['canvas']
    render(wrap(<CanvasLink projectId="p1" />))
    expect(screen.getByRole('link', { name: 'Canvas' })).toBeTruthy()
  })

  it('redirects the route to Beta features when off, without any project request', async () => {
    const client = new QueryClient()
    client.setQueryData(devModeQueryOptions().queryKey, {
      enabled: false,
      experimental: [],
    })
    const err: unknown = await requireExperimental(
      client,
      'canvas',
      '/projects/p1/canvas',
    ).catch((e: unknown) => e)
    expect(isRedirect(err)).toBe(true)
    await expect(
      requireExperimental(
        (() => {
          const c = new QueryClient()
          c.setQueryData(devModeQueryOptions().queryKey, {
            enabled: false,
            experimental: ['canvas'],
          })
          return c
        })(),
        'canvas',
      ),
    ).resolves.toBeUndefined()
  })
})

const plan: IacPlan = {
  hash: 'h',
  summary: { create: 1, update: 0, delete: 0, noop: 0, error: 0 },
  changes: [{ kind: 'App', name: 'api', action: 'create' }],
}

function props(
  over: Partial<CanvasReviewDialogProps> = {},
): CanvasReviewDialogProps {
  return {
    open: true,
    hasServerPlan: true,
    plan,
    planning: false,
    planError: null,
    applyError: null,
    applying: false,
    outcome: null,
    resolved: { steps: [], unsupported: [] },
    lookupsLoading: false,
    onClose: vi.fn(),
    onRetry: vi.fn(),
    onApply: vi.fn(),
    ...over,
  }
}

describe('CanvasReviewDialog', () => {
  it('shows the server plan and applies on request', async () => {
    const p = props()
    render(wrap(<CanvasReviewDialog {...p} />))
    expect(screen.getByText('App/api')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Apply' }))
    expect(p.onApply).toHaveBeenCalled()
  })

  it('shows validation issues inline with a next action and no Apply', () => {
    const planError = new IacIssuesError([
      { file: 'canvas-app-api.yaml', line: 4, message: 'port is invalid' },
    ])
    render(wrap(<CanvasReviewDialog {...props({ plan: null, planError })} />))
    expect(
      screen.getByText(/canvas-app-api.yaml:4 port is invalid/),
    ).toBeTruthy()
    expect(screen.getByText(/fix or remove the item named above/)).toBeTruthy()
    expect(
      screen.getByRole('button', { name: 'Apply' }).hasAttribute('disabled'),
    ).toBe(true)
  })

  it('offers retry on a server error', async () => {
    const p = props({ plan: null, planError: new ApiError(500, 'boom') })
    render(wrap(<CanvasReviewDialog {...p} />))
    expect(screen.getByText('boom')).toBeTruthy()
    await userEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(p.onRetry).toHaveBeenCalled()
  })

  it('blocks Apply when the plan carries errors', () => {
    const bad: IacPlan = {
      ...plan,
      summary: { ...plan.summary, error: 1 },
    }
    render(wrap(<CanvasReviewDialog {...props({ plan: bad })} />))
    expect(
      screen.getByRole('button', { name: 'Apply' }).hasAttribute('disabled'),
    ).toBe(true)
  })

  it('lists connection steps the server does not preview', () => {
    render(
      wrap(
        <CanvasReviewDialog
          {...props({
            hasServerPlan: false,
            plan: null,
            resolved: {
              steps: [
                { kind: 'connect', app: 'web', database: 'main', envVar: '' },
              ],
              unsupported: [{ app: 'w2', database: 'main' }],
            },
          })}
        />,
      ),
    )
    expect(screen.getByText('Connect web to main')).toBeTruthy()
    expect(screen.getByText(/does not preview them/)).toBeTruthy()
    expect(screen.getByText(/Edit app.yaml instead/)).toBeTruthy()
  })
})
