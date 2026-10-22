import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { COPY_FEEDBACK_MS } from '@/lib/copyFeedback'
import { CommonI18n } from '../../test/commonI18n'
import { CodeBlock, CommandBlock } from './code-block'

let writeText: ReturnType<typeof vi.fn>

beforeEach(() => {
  vi.useFakeTimers()
  writeText = vi.fn().mockResolvedValue(undefined)
  Object.defineProperty(navigator, 'clipboard', {
    value: { writeText },
    configurable: true,
  })
})

afterEach(() => {
  vi.useRealTimers()
})

async function click(name: RegExp) {
  fireEvent.click(screen.getByRole('button', { name }))
  await act(() => Promise.resolve())
}

describe('CodeBlock', () => {
  it('copies the code and announces Copied politely, then resets', async () => {
    render(
      <CommonI18n>
        <CodeBlock code={'a: 1\nb: 2'} language="yaml" />
      </CommonI18n>,
    )
    await click(/^copy$/i)
    expect(writeText).toHaveBeenCalledWith('a: 1\nb: 2')
    const status = screen.getByRole('status')
    expect(status).toHaveTextContent('Copied')
    expect(status).toHaveAttribute('aria-live', 'polite')
    act(() => {
      vi.advanceTimersByTime(COPY_FEEDBACK_MS)
    })
    expect(status).toBeEmptyDOMElement()
  })

  it('announces a failure when the clipboard rejects', async () => {
    writeText.mockRejectedValue(new Error('denied'))
    render(
      <CommonI18n>
        <CodeBlock code="x" />
      </CommonI18n>,
    )
    await click(/^copy$/i)
    expect(screen.getByRole('status')).toHaveTextContent('Copy failed')
  })

  it('toggles wrapping', () => {
    render(
      <CommonI18n>
        <CodeBlock code="long line" label="Output" />
      </CommonI18n>,
    )
    const region = screen.getByLabelText('Output')
    expect(region).toHaveClass('whitespace-pre')
    fireEvent.click(screen.getByRole('button', { name: /wrap/i }))
    expect(region).toHaveClass('whitespace-pre-wrap')
  })

  it('is a focusable named scroll region', () => {
    render(
      <CommonI18n>
        <CodeBlock code="x" label="Config" />
      </CommonI18n>,
    )
    expect(screen.getByLabelText('Config')).toHaveAttribute('tabindex', '0')
  })

  it('does not copy the prompt', async () => {
    render(
      <CommonI18n>
        <CodeBlock code={'$ ls\n$ pwd'} prompt />
      </CommonI18n>,
    )
    await click(/^copy$/i)
    expect(writeText).toHaveBeenCalledWith('ls\npwd')
  })

  it('shows line numbers only when asked', () => {
    const { rerender } = render(
      <CommonI18n>
        <CodeBlock code={'a\nb'} label="L" />
      </CommonI18n>,
    )
    expect(screen.getByLabelText('L').textContent).toBe('a\nb')
    rerender(
      <CommonI18n>
        <CodeBlock code={'a\nb'} label="L" lineNumbers />
      </CommonI18n>,
    )
    expect(screen.getByLabelText('L').textContent).toContain('2')
  })

  it('masks secrets until revealed but copies the real value', async () => {
    render(
      <CommonI18n>
        <CodeBlock code="tok_abc123" label="Key" secret />
      </CommonI18n>,
    )
    expect(screen.getByLabelText('Key')).not.toHaveTextContent('tok_abc123')
    await click(/^copy$/i)
    expect(writeText).toHaveBeenCalledWith('tok_abc123')
    fireEvent.click(screen.getByRole('button', { name: /reveal/i }))
    expect(screen.getByLabelText('Key')).toHaveTextContent('tok_abc123')
    expect(screen.getByRole('button', { name: /hide/i })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
  })

  it('clears the reset timer on unmount', async () => {
    const { unmount } = render(
      <CommonI18n>
        <CodeBlock code="x" />
      </CommonI18n>,
    )
    await click(/^copy$/i)
    const before = vi.getTimerCount()
    expect(before).toBeGreaterThan(0)
    unmount()
    expect(vi.getTimerCount()).toBe(0)
  })
})

describe('CommandBlock', () => {
  it('shows a prompt that is not copied', async () => {
    render(
      <CommonI18n>
        <CommandBlock code="levelrail-cli auth login" label="Login" />
      </CommonI18n>,
    )
    expect(screen.getByLabelText('Login')).toHaveTextContent(
      '$ levelrail-cli auth login',
    )
    await click(/^copy$/i)
    expect(writeText).toHaveBeenCalledWith('levelrail-cli auth login')
    expect(screen.getByRole('status')).toHaveTextContent('Copied')
  })

  it('masks a secret command', () => {
    render(
      <CommonI18n>
        <CommandBlock code="export T=abc" label="Env" secret />
      </CommonI18n>,
    )
    expect(screen.getByLabelText('Env')).not.toHaveTextContent('abc')
  })
})
