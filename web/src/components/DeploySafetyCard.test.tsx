import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '../test/commonI18n'
import { DeploySafetyCard } from './DeploySafetyCard'

let pending: boolean
vi.mock('../queries/deployGate', () => ({
  useDeploySafety: () => ({
    isPending: pending,
    isError: false,
    data: undefined,
  }),
}))
vi.mock('./RollbackTargetsList', () => ({
  RollbackTargetsList: () => <p>targets-list</p>,
}))

describe('DeploySafetyCard', () => {
  beforeEach(() => {
    pending = true
  })

  it('reserves the card with a skeleton while pending', () => {
    render(
      <CommonI18n>
        <DeploySafetyCard appName="web" />
      </CommonI18n>,
    )
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.queryByText('targets-list')).toBeNull()
  })

  it('renders the real card after the query settles', () => {
    pending = false
    render(
      <CommonI18n>
        <DeploySafetyCard appName="web" />
      </CommonI18n>,
    )
    expect(screen.queryByRole('status')).toBeNull()
    expect(screen.getByText('targets-list')).toBeInTheDocument()
  })
})
