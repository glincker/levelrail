import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BrandIcon, type BrandIconName } from './BrandIcon'

describe('BrandIcon', () => {
  it('renders a blank placeholder for a name it has no mark for', () => {
    const { container } = render(
      <BrandIcon name={'not-a-brand' as BrandIconName} className="size-4" />,
    )
    expect(container.querySelector('span.size-4')).not.toBeNull()
  })
})
