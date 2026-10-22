import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../test/commonI18n'
import { CancelSupersededCard } from './CancelSupersededCard'

type SettingState = { isPending: boolean; data?: { enabled: boolean } }
let state: SettingState
vi.mock('../queries/deployControl', () => ({
  useCancelSuperseded: () => state,
  useSetCancelSuperseded: () => ({ isPending: false, mutate: vi.fn() }),
}))

function renderCard() {
  return render(
    <CommonI18n>
      <CancelSupersededCard appName="web" />
    </CommonI18n>,
  )
}

describe('CancelSupersededCard', () => {
  beforeEach(() => {
    state = { isPending: true }
  })

  it('shows a placeholder instead of an off switch while pending', () => {
    renderCard()
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.queryByRole('switch')).toBeNull()
  })

  it('renders the saved switch state once data arrives', () => {
    const view = renderCard()
    state = { isPending: false, data: { enabled: true } }
    view.rerender(
      <CommonI18n>
        <CancelSupersededCard appName="web" />
      </CommonI18n>,
    )
    expect(screen.queryByRole('status')).toBeNull()
    expect(
      screen.getByRole('switch', { name: 'Replace superseded queued deploys' }),
    ).toBeChecked()
  })
})
