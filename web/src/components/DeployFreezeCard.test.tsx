import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../test/commonI18n'
import { DeployFreezeCard } from './DeployFreezeCard'

type FreezeState = {
  isPending: boolean
  isError: boolean
  data?: { windows: []; status: { frozen: boolean } }
}
let state: FreezeState
vi.mock('../queries/deployFreeze', () => ({
  useDeployFreezeOptional: () => state,
  useSetDeployFreeze: () => ({ isPending: false, mutate: vi.fn() }),
}))

// HelpLink needs a BrandProvider, which this test does not exercise.
vi.mock('@/components/HelpLink', () => ({ HelpLink: () => null }))

function renderCard() {
  return render(
    <CommonI18n>
      <DeployFreezeCard appName="web" />
    </CommonI18n>,
  )
}

describe('DeployFreezeCard', () => {
  beforeEach(() => {
    state = { isPending: true, isError: false }
  })

  it('shows a card skeleton while pending, never nothing', () => {
    renderCard()
    expect(screen.getByRole('status')).toBeInTheDocument()
  })

  it('swaps the skeleton for the real card once data arrives', () => {
    const view = renderCard()
    state = {
      isPending: false,
      isError: false,
      data: { windows: [], status: { frozen: false } },
    }
    view.rerender(
      <CommonI18n>
        <DeployFreezeCard appName="web" />
      </CommonI18n>,
    )
    expect(screen.queryByRole('status')).toBeNull()
    expect(view.container.querySelector('[data-slot="card"]')).not.toBeNull()
  })

  it('renders nothing only after an error', () => {
    state = { isPending: false, isError: true }
    const view = renderCard()
    expect(view.container).toBeEmptyDOMElement()
  })
})
