import { describe, expect, it } from 'vitest'
import { md5, ociFingerprint } from './ociKey'

const enc = (s: string) => new TextEncoder().encode(s)
const hex = (b: Uint8Array) =>
  Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')

describe('ociFingerprint', () => {
  it('matches RFC 1321 MD5 vectors', () => {
    expect(hex(md5(enc('')))).toBe('d41d8cd98f00b204e9800998ecf8427e')
    expect(hex(md5(enc('abc')))).toBe('900150983cd24fb0d6963f7d28e17f72')
    expect(
      hex(
        md5(
          enc(
            '12345678901234567890123456789012345678901234567890123456789012345678901234567890',
          ),
        ),
      ),
    ).toBe('57edf4a22be3c955ac49da2e2107b67a')
  })

  it('formats as colon separated hex', () => {
    expect(ociFingerprint(enc('abc'))).toBe(
      '90:01:50:98:3c:d2:4f:b0:d6:96:3f:7d:28:e1:7f:72',
    )
  })
})
