import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import { clearStoredUsername, setStoredUsername } from '../../lib/authStore'
import stacksEn from '../../locales/en/stacks.json'
import { ApproveDialog } from './ApprovalDialogs'
import { RevisionHistory } from './RevisionHistory'
import { RevisionView } from './RevisionView'
import { canRollbackTo } from './approval'
import {
  jsonResponse,
  renderWithProviders,
  revision,
  stack,
} from './stacksTestUtils'

beforeEach(() => {
  setStoredUsername('root@example.com')
})
afterEach(() => {
  vi.unstubAllGlobals()
  clearStoredUsername()
})

function detail(result: unknown, status = 'applied') {
  return { ...revision(2, { status: status as 'applied' }), files: [], result }
}

function renderView(body: unknown, against: number | 'live' = 'live') {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) =>
      Promise.resolve(
        jsonResponse(
          url.includes('/diff')
            ? {
                stack: 'shop',
                revision: 2,
                against: 1,
                changes: [
                  { kind: 'app', name: 'web', action: 'changed' },
                  { kind: 'app', name: 'api', action: 'added' },
                  { kind: 'app', name: 'old', action: 'removed' },
                ],
              }
            : body,
        ),
      ),
    ),
  )
  return renderWithProviders(
    <RevisionView
      name="shop"
      revision={2}
      against={against}
      otherRevisions={[1]}
      showDiff={against !== 'live'}
      onAgainst={vi.fn()}
      onClose={vi.fn()}
    />,
  )
}

describe('RevisionView result', () => {
  it('summarises counts and failed items from the apply result', async () => {
    renderView(
      detail({
        applied: 2,
        failed: 1,
        skipped: 0,
        results: [
          { kind: 'app', name: 'web', status: 'applied' },
          { kind: 'app', name: 'api', status: 'failed', error: 'port in use' },
        ],
      }),
    )
    expect(
      await screen.findByText('2 applied, 1 failed, 0 skipped'),
    ).toBeInTheDocument()
    expect(screen.getByText('app api: port in use')).toBeInTheDocument()
    expect(screen.queryByText('No result yet.')).not.toBeInTheDocument()
  })

  it('says no result only when the result is absent', async () => {
    renderView(detail(undefined, 'pending_approval'))
    expect(await screen.findByText('No result yet.')).toBeInTheDocument()
  })

  it('labels revision to revision changes instead of printing keys', async () => {
    renderView(detail({ applied: 1 }), 1)
    expect(await screen.findByText('Changed')).toBeInTheDocument()
    expect(screen.getByText('Added')).toBeInTheDocument()
    expect(screen.getByText('Removed')).toBeInTheDocument()
    expect(screen.queryByText(/diff\.action/)).not.toBeInTheDocument()
  })
})

describe('rollback targets', () => {
  it('allows only ran revisions that are not the latest', () => {
    expect(canRollbackTo(revision(2, { status: 'applied' }), 3)).toBe(true)
    expect(canRollbackTo(revision(2, { status: 'superseded' }), 3)).toBe(true)
    expect(canRollbackTo(revision(2, { status: 'partial' }), 3)).toBe(true)
    expect(canRollbackTo(revision(2, { status: 'pending_approval' }), 3)).toBe(
      false,
    )
    expect(canRollbackTo(revision(2, { status: 'failed' }), 3)).toBe(false)
    expect(
      canRollbackTo(revision(2, { status: 'failed', rejected: true }), 3),
    ).toBe(false)
    expect(canRollbackTo(revision(3, { status: 'applied' }), 3)).toBe(false)
  })

  it('disables the button for pending revisions in history', () => {
    renderWithProviders(
      <RevisionHistory
        stack={stack({ latest_revision: 4 })}
        isRoot
        currentUser="root@example.com"
        revisions={[
          revision(3, { status: 'pending_approval' }),
          revision(2, { status: 'pending_approval' }),
          revision(1, { status: 'applied' }),
        ]}
        onView={vi.fn()}
        onDiff={vi.fn()}
        onRollback={vi.fn()}
        onPin={vi.fn()}
        onApprove={vi.fn()}
        onReject={vi.fn()}
      />,
    )
    expect(
      screen.getAllByLabelText('Roll back to revision 3')[0],
    ).toBeDisabled()
    expect(
      screen.getAllByLabelText('Roll back to revision 2')[0],
    ).toBeDisabled()
    expect(screen.getAllByLabelText('Roll back to revision 1')[0]).toBeEnabled()
  })
})

describe('ApproveDialog', () => {
  it('blocks the requester with an accurate message', () => {
    renderWithProviders(
      <ApproveDialog
        name="shop"
        revision={revision(3, {
          status: 'pending_approval',
          actor: 'root@example.com',
        })}
        onClose={vi.fn()}
      />,
    )
    expect(screen.getByText(/You requested this revision/)).toBeInTheDocument()
    expect(screen.queryByText(/Another admin requested it/)).toBeNull()
    expect(screen.getByRole('button', { name: 'Approve' })).toBeDisabled()
  })

  it('never falls back to revision 0 in the title while closing', () => {
    const rev = revision(3, { status: 'pending_approval', actor: 'ada' })
    function Harness() {
      const [open, setOpen] = useState(true)
      return (
        <>
          <button
            type="button"
            onClick={() => {
              setOpen(false)
            }}
          >
            parent-close
          </button>
          <ApproveDialog
            name="shop"
            revision={open ? rev : null}
            onClose={vi.fn()}
          />
        </>
      )
    }
    renderWithProviders(<Harness />)
    expect(screen.getByText('Approve revision 3?')).toBeInTheDocument()
    fireEvent.click(screen.getByText('parent-close'))
    expect(screen.queryByText('Approve revision 0?')).toBeNull()
  })

  it('asks the parent to close on Cancel and Escape', async () => {
    const onClose = vi.fn()
    renderWithProviders(
      <ApproveDialog
        name="shop"
        revision={revision(3, { status: 'pending_approval', actor: 'ada' })}
        onClose={onClose}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => {
      expect(onClose).toHaveBeenCalledTimes(1)
    })
    fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' })
    await waitFor(() => {
      expect(onClose).toHaveBeenCalledTimes(2)
    })
  })
})

describe('stacks locale', () => {
  it('defines every t() key used by the stacks components', () => {
    const dir = __dirname
    const keys = new Set<string>()
    for (const f of readdirSync(dir)) {
      if (!/\.tsx?$/.test(f) || /\.test\./.test(f)) continue
      const src = readFileSync(join(dir, f), 'utf8')
      for (const m of src.matchAll(/\bt\(\s*'([\w.]+)'/g)) keys.add(m[1] ?? '')
      for (const m of src.matchAll(/'((?:\w+\.)+\w+)'/g)) {
        const key = m[1] ?? ''
        if (/^(approval|revision|diff|actions|detail)\./.test(key)) {
          keys.add(key)
        }
      }
    }
    // Dynamic keys: diff.action.<IacAction | diff action>, status.<status>.
    for (const a of [
      'create',
      'update',
      'delete',
      'noop',
      'error',
      'added',
      'removed',
      'changed',
    ]) {
      keys.add(`diff.action.${a}`)
    }
    const lookup = (key: string): unknown =>
      key
        .split('.')
        .reduce<unknown>(
          (o, p) => (o as Record<string, unknown> | undefined)?.[p],
          stacksEn,
        )
    const missing = [...keys].filter((k) => typeof lookup(k) !== 'string')
    expect(missing).toEqual([])
  })
})
