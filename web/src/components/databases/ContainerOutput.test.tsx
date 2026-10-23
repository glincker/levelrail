import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ContainerOutput } from './ContainerOutput'

describe('ContainerOutput', () => {
  it('renders the cause and a collapsed output block', () => {
    render(
      <ContainerOutput
        failure={{
          step: 'wait_ready',
          reason: 'not running',
          log_tail: 'setpriv: Operation not permitted',
          cause: { kind: 'permission', hint: 'fallback hint' },
        }}
      />,
    )
    expect(screen.getByText('setpriv: Operation not permitted')).toBeTruthy()
    expect(screen.getByText(/containerOutput|Container output/)).toBeTruthy()
  })

  it('renders nothing without cause or output', () => {
    const { container } = render(
      <ContainerOutput failure={{ step: 'start', reason: 'x' }} />,
    )
    expect(container.firstChild).toBeNull()
  })
})
