import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { LockedFeatureMark } from './LockedFeatureMark'

describe('LockedFeatureMark', () => {
  it('keeps the badge and description without an extra icon', () => {
    const { container } = render(<LockedFeatureMark id="why" />)
    expect(screen.getByText('shell.locked.badge')).toHaveClass('ml-auto')
    expect(container.querySelector('svg')).toBeNull()
    expect(container.querySelector('#why')).not.toBeNull()
  })
})
