import { describe, expect, it } from 'vitest'
import { isIOSSafari } from './pwaInstall'

describe('isIOSSafari', () => {
  it('detects Safari on iPhone and rejects other iOS browsers', () => {
    expect(
      isIOSSafari(
        'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1',
      ),
    ).toBe(true)
    expect(
      isIOSSafari(
        'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 CriOS/120.0 Mobile/15E148 Safari/604.1',
      ),
    ).toBe(false)
    expect(isIOSSafari('Mozilla/5.0 (X11; Linux x86_64) Chrome/120')).toBe(
      false,
    )
  })
})
