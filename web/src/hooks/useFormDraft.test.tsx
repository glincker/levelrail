import { act } from 'react'
import { render, screen } from '@testing-library/react'
import { fireEvent } from '@testing-library/react'
import { useForm } from 'react-hook-form'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useFormDraft } from './useFormDraft'

interface DraftFormValues extends Record<string, unknown> {
  name: string
  token: string
}

const DEFAULT_VALUES: DraftFormValues = { name: '', token: '' }

function readStoredDraft(key: string): unknown {
  const raw = window.localStorage.getItem(key)
  return raw === null ? null : (JSON.parse(raw) as unknown)
}

function DraftForm({
  storageKey,
  open,
  excludeKeys,
}: {
  storageKey: string
  open: boolean
  excludeKeys?: readonly (keyof DraftFormValues)[]
}) {
  const { register, watch, reset } = useForm<DraftFormValues>({
    defaultValues: DEFAULT_VALUES,
  })
  const { restoredFromDraft, discardDraft, dismissDraftNotice, clearDraft } =
    useFormDraft({
      storageKey,
      open,
      watch,
      reset,
      defaultValues: DEFAULT_VALUES,
      excludeKeys,
    })

  return (
    <div>
      <input aria-label="name" {...register('name')} />
      <input aria-label="token" {...register('token')} />
      {restoredFromDraft ? <p>Restored your unsaved draft from earlier.</p> : null}
      <button type="button" onClick={discardDraft}>
        Discard draft
      </button>
      <button type="button" onClick={dismissDraftNotice}>
        Dismiss
      </button>
      <button type="button" onClick={clearDraft}>
        Clear (simulated submit)
      </button>
    </div>
  )
}

describe('useFormDraft', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    window.localStorage.clear()
  })

  it('saves values to localStorage after they settle, debounced', () => {
    render(<DraftForm storageKey="test-draft" open />)

    fireEvent.change(screen.getByLabelText('name'), {
      target: { value: 'demo-app' },
    })

    expect(window.localStorage.getItem('test-draft')).toBeNull()

    act(() => {
      vi.advanceTimersByTime(700)
    })

    expect(readStoredDraft('test-draft')).toEqual({
      name: 'demo-app',
      token: '',
    })
  })

  it('never persists an excluded field, even once other fields are saved', () => {
    render(
      <DraftForm storageKey="test-draft-secret" open excludeKeys={['token']} />,
    )

    fireEvent.change(screen.getByLabelText('name'), {
      target: { value: 'demo-app' },
    })
    fireEvent.change(screen.getByLabelText('token'), {
      target: { value: 'super-secret-token' },
    })

    act(() => {
      vi.advanceTimersByTime(700)
    })

    const saved = readStoredDraft('test-draft-secret')
    expect(saved).toEqual({ name: 'demo-app' })
    expect(JSON.stringify(saved)).not.toContain('super-secret-token')
  })

  it('clears the draft once every field is back to its default', () => {
    render(<DraftForm storageKey="test-draft-clear" open />)

    fireEvent.change(screen.getByLabelText('name'), {
      target: { value: 'demo-app' },
    })
    act(() => {
      vi.advanceTimersByTime(700)
    })
    expect(window.localStorage.getItem('test-draft-clear')).not.toBeNull()

    fireEvent.change(screen.getByLabelText('name'), { target: { value: '' } })
    act(() => {
      vi.advanceTimersByTime(700)
    })
    expect(window.localStorage.getItem('test-draft-clear')).toBeNull()
  })

  it('restores a saved draft on a fresh mount and shows the notice', () => {
    window.localStorage.setItem(
      'test-draft-restore',
      JSON.stringify({ name: 'restored-app', token: '' }),
    )

    render(<DraftForm storageKey="test-draft-restore" open />)

    // The restore effect is a layout effect, flushed synchronously by
    // render()'s own act() wrapper, so no findBy/waitFor is needed here.
    screen.getByText('Restored your unsaved draft from earlier.')
    expect(screen.getByLabelText('name')).toHaveValue('restored-app')
  })

  it('discarding the draft clears storage and resets the form to blank', () => {
    window.localStorage.setItem(
      'test-draft-discard',
      JSON.stringify({ name: 'restored-app', token: '' }),
    )

    render(<DraftForm storageKey="test-draft-discard" open />)
    screen.getByText('Restored your unsaved draft from earlier.')

    fireEvent.click(screen.getByRole('button', { name: 'Discard draft' }))

    expect(window.localStorage.getItem('test-draft-discard')).toBeNull()
    expect(screen.getByLabelText('name')).toHaveValue('')
    expect(
      screen.queryByText('Restored your unsaved draft from earlier.'),
    ).not.toBeInTheDocument()
  })

  it('dismissing the notice hides it without discarding the restored values', () => {
    window.localStorage.setItem(
      'test-draft-dismiss',
      JSON.stringify({ name: 'restored-app', token: '' }),
    )

    render(<DraftForm storageKey="test-draft-dismiss" open />)
    screen.getByText('Restored your unsaved draft from earlier.')

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))

    expect(
      screen.queryByText('Restored your unsaved draft from earlier.'),
    ).not.toBeInTheDocument()
    // Dismissing is not discarding: the restored value and the saved
    // draft both stay put.
    expect(screen.getByLabelText('name')).toHaveValue('restored-app')
    expect(window.localStorage.getItem('test-draft-dismiss')).not.toBeNull()
  })

  it('clearDraft (a successful submit) removes the draft without touching form values', () => {
    render(<DraftForm storageKey="test-draft-submit" open />)

    fireEvent.change(screen.getByLabelText('name'), {
      target: { value: 'demo-app' },
    })
    act(() => {
      vi.advanceTimersByTime(700)
    })
    expect(window.localStorage.getItem('test-draft-submit')).not.toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Clear (simulated submit)' }))

    expect(window.localStorage.getItem('test-draft-submit')).toBeNull()
    expect(screen.getByLabelText('name')).toHaveValue('demo-app')
  })

  it('does not restore a draft while closed, and re-checks the next time it opens', () => {
    window.localStorage.setItem(
      'test-draft-reopen',
      JSON.stringify({ name: 'restored-app', token: '' }),
    )

    const { rerender } = render(
      <DraftForm storageKey="test-draft-reopen" open={false} />,
    )
    expect(
      screen.queryByText('Restored your unsaved draft from earlier.'),
    ).not.toBeInTheDocument()

    rerender(<DraftForm storageKey="test-draft-reopen" open />)
    screen.getByText('Restored your unsaved draft from earlier.')
  })
})
