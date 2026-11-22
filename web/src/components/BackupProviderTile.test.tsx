import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { BackupProviderTile } from './BackupProviderTile'

function tile(provider: Parameters<typeof BackupProviderTile>[0]['provider']) {
  const { container } = render(
    <BackupProviderTile provider={provider} label="Provider" />,
  )
  return container.querySelector('[data-slot="brand-tile"]') as HTMLElement
}

describe('BackupProviderTile', () => {
  it.each(['custom', 'sftp', 'local'] as const)(
    'draws a glyph, not an initial, for %s',
    (provider) => {
      const el = tile(provider)
      expect(el.querySelector('svg')).not.toBeNull()
      expect(el.textContent).toBe('')
    },
  )
})
