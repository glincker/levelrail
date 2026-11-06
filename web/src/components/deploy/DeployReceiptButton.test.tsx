import { describe, expect, it } from 'vitest'
import { deployReceiptUrl } from './deployReceiptUrl'

describe('deployReceiptUrl', () => {
  it('escapes ids and selects the format', () => {
    expect(deployReceiptUrl('web', 'dep 1', 'md')).toBe(
      '/api/v1/apps/web/deployments/dep%201/receipt?format=md',
    )
  })
})
