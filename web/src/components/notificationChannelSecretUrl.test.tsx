import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CreateNotificationChannelDialog } from './CreateNotificationChannelDialog'
import { EditNotificationChannelDialog } from './EditNotificationChannelDialog'
import type { NotificationChannel } from '../types/notificationChannel'
import type { FieldDescriptor } from '../types/fieldDescriptor'

const createMutate = vi.fn()
const updateMutate = vi.fn()
const idle = { mutate: vi.fn(), reset: vi.fn(), isPending: false }

vi.mock('../queries/notificationChannels', () => ({
  useCreateNotificationChannel: () => ({
    mutate: createMutate,
    reset: vi.fn(),
    isPending: false,
  }),
  useUpdateNotificationChannel: () => ({
    mutate: updateMutate,
    reset: vi.fn(),
    isPending: false,
  }),
  useTestNotificationChannel: () => idle,
  useTestExistingNotificationChannel: () => idle,
}))
vi.mock('./ChannelQuietHours', () => ({ ChannelQuietHours: () => null }))

const base = { type: 'string', required: true, doc: '', help_key: 'h' } as const
const webhookKinds = ['slack', 'generic']
const fields: FieldDescriptor[] = [
  { ...base, id: 'name', key: 'name', secret: false, label_key: 'Name' },
  {
    ...base,
    id: 'notify_url_webhook',
    key: 'notify_url',
    secret: true,
    label_key: 'Destination',
    show_when: [{ field: 'kind', equals: webhookKinds }],
  },
]

const MASKED = 'https://hooks.example.com/***'

const channel: NotificationChannel = {
  id: 'chn_1',
  name: 'Team',
  kind: 'generic',
  notify_url: MASKED,
  enabled: true,
  interactive_approvals: false,
  has_interactive_secret: false,
  notify_device_login: false,
  notify_device_login_expired: false,
  created_at: '',
  updated_at: '',
}

function renderWithClient(ui: React.ReactElement) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>)
}

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn<typeof fetch>(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ name: 'notification-channels', fields }),
      } as unknown as Response),
    ),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  createMutate.mockReset()
  updateMutate.mockReset()
})

describe('notification channel secret URL fields', () => {
  it('edit: masked value is shown hidden and saved unchanged so the server keeps it', async () => {
    renderWithClient(<EditNotificationChannelDialog channel={channel} />)
    fireEvent.click(screen.getByText('Edit'))
    const input = await waitFor(() => {
      const el = document.querySelector<HTMLInputElement>(
        '#edit-channel-notify_url_webhook',
      )
      if (!el) throw new Error('not rendered')
      return el
    })
    expect(input.type).toBe('password')
    expect(input.value).toBe(MASKED)
    fireEvent.click(screen.getByText('Save changes'))
    await waitFor(() => expect(updateMutate).toHaveBeenCalled())
    const sent = updateMutate.mock.calls[0]?.[0] as {
      req: { notify_url: string }
    }
    expect(sent.req.notify_url).toBe(MASKED)
  })

  it('edit: a typed replacement is sent instead of the mask', async () => {
    renderWithClient(<EditNotificationChannelDialog channel={channel} />)
    fireEvent.click(screen.getByText('Edit'))
    const input = await waitFor(() => {
      const el = document.querySelector<HTMLInputElement>(
        '#edit-channel-notify_url_webhook',
      )
      if (!el) throw new Error('not rendered')
      return el
    })
    fireEvent.change(input, { target: { value: 'https://new.example.com/x' } })
    fireEvent.click(screen.getByText('Save changes'))
    await waitFor(() => expect(updateMutate).toHaveBeenCalled())
    const sent = updateMutate.mock.calls[0]?.[0] as {
      req: { notify_url: string }
    }
    expect(sent.req.notify_url).toBe('https://new.example.com/x')
  })

  it('create: the secret URL input starts blank, hidden, and submits what was typed', async () => {
    renderWithClient(<CreateNotificationChannelDialog />)
    fireEvent.click(screen.getByText('Connect channel'))
    const input = await waitFor(() => {
      const el = document.querySelector<HTMLInputElement>(
        '#channel-notify_url_webhook',
      )
      if (!el) throw new Error('not rendered')
      return el
    })
    expect(input.type).toBe('password')
    expect(input.value).toBe('')
    const name = document.querySelector<HTMLInputElement>('#channel-name')
    if (!name) throw new Error('name missing')
    fireEvent.change(name, { target: { value: 'Ops' } })
    fireEvent.change(input, {
      target: { value: 'https://hooks.example.com/a' },
    })
    const submit = screen
      .getAllByText('Connect channel')
      .map((el) => el.closest('button'))
      .find((b) => b?.getAttribute('type') === 'submit')
    if (!submit) throw new Error('submit missing')
    fireEvent.click(submit)
    await waitFor(() => expect(createMutate).toHaveBeenCalled())
    const sent = createMutate.mock.calls[0]?.[0] as { notify_url: string }
    expect(sent.notify_url).toBe('https://hooks.example.com/a')
  })
})
