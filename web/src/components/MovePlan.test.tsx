import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MovePlan } from './MovePlan'
import { moveSteps } from '../lib/moveSteps'

describe('MovePlan', () => {
  it('lists stop, one copy per volume, placement and start in order', () => {
    expect(moveSteps('web', ['data', 'cache'])).toEqual([
      'Stop web on its current node',
      'Copy volume data to the destination',
      'Copy volume cache to the destination',
      'Point the app at the destination node',
      'Start web there',
    ])
  })

  it('states the downtime and no-rollback facts', () => {
    render(<MovePlan appName="web" volumeNames={['data']} />)
    expect(screen.getByText('Downtime')).toBeInTheDocument()
    expect(
      screen.getByText(/no automatic health gate or rollback/),
    ).toBeInTheDocument()
  })
})
