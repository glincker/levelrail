import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../../test/commonI18n'
import { SessionDeviceRow } from './SessionDeviceRow'
import type { SessionEntry } from '../../queries/securityCenter'

const base: SessionEntry = {
  id: 's1',
  browser: 'Firefox on Windows',
  ip: '203.0.113.9',
  network: '203.0.113.0/24',
  created_at: '2026-10-15T10:00:00Z',
  last_seen_at: '2026-10-15T11:58:00Z',
  expires_at: '2026-10-22T10:00:00Z',
  current: false,
  can_approve: true,
}

function renderRow(s: SessionEntry) {
  return render(
    <CommonI18n>
      <ul>
        <SessionDeviceRow
          session={s}
          now={Date.parse('2026-10-15T12:00:00Z')}
          busy={false}
          onRevoke={vi.fn()}
        />
      </ul>
    </CommonI18n>,
  )
}

describe('SessionDeviceRow', () => {
  it('shows the device, address and an approve hint for another live session', () => {
    renderRow(base)
    expect(screen.getByText('Firefox on Windows')).toBeInTheDocument()
    expect(screen.getByText('203.0.113.9')).toBeInTheDocument()
    expect(screen.getByRole('button')).toBeEnabled()
  })

  it('offers no sign out for the current session', () => {
    renderRow({ ...base, current: true })
    expect(screen.queryByRole('button')).toBeNull()
  })
})
