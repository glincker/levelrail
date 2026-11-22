import { describe, expect, it } from 'vitest'
import { OCI_RE, parseOciConfig, shortOcid } from './ociConfig'

const FP = 'aa:bb:cc:dd:ee:ff:00:11:22:33:44:55:66:77:88:99'

describe('parseOciConfig', () => {
  it('reads the DEFAULT block', () => {
    const c = parseOciConfig(
      `[DEFAULT]\nuser=ocid1.user.oc1..u\nfingerprint=${FP}\ntenancy=ocid1.tenancy.oc1..t\nregion=us-phoenix-1\nkey_file=~/.oci/k.pem\n`,
    )
    expect(c).toEqual({
      user: 'ocid1.user.oc1..u',
      fingerprint: FP,
      tenancy: 'ocid1.tenancy.oc1..t',
      region: 'us-phoenix-1',
      keyFile: '~/.oci/k.pem',
    })
  })

  it('prefers DEFAULT over other profiles regardless of order', () => {
    const c = parseOciConfig(
      '[OTHER]\nregion=eu-frankfurt-1\n[DEFAULT]\nregion=us-phoenix-1\n',
    )
    expect(c.region).toBe('us-phoenix-1')
  })

  it('falls back to the first profile when there is no DEFAULT', () => {
    const c = parseOciConfig('[ONE]\nregion=ap-tokyo-1\n[TWO]\nregion=x\n')
    expect(c.region).toBe('ap-tokyo-1')
  })

  it('returns empty strings for missing keys and empty input', () => {
    expect(parseOciConfig('[DEFAULT]\nregion=us-ashburn-1').user).toBe('')
    expect(parseOciConfig('')).toMatchObject({ tenancy: '', region: '' })
  })

  it('tolerates whitespace, CRLF, quotes and comments', () => {
    const c = parseOciConfig(
      '# note\r\n[ DEFAULT ]\r\n  region  =  "us-phoenix-1"  \r\n; x\r\n tenancy = \'ocid1.tenancy.oc1..t\'\r\n',
    )
    expect(c.region).toBe('us-phoenix-1')
    expect(c.tenancy).toBe('ocid1.tenancy.oc1..t')
  })

  it('accepts a header-less snippet', () => {
    expect(parseOciConfig('region=us-phoenix-1').region).toBe('us-phoenix-1')
  })
})

describe('helpers', () => {
  it('shortens long OCIDs only', () => {
    expect(shortOcid('ocid1.tenancy.oc1..aaaaaaabcdef')).toBe('ocid1...abcdef')
    expect(shortOcid('short')).toBe('short')
  })

  it('validates shapes', () => {
    expect(OCI_RE.fingerprint.test(FP)).toBe(true)
    expect(OCI_RE.fingerprint.test('aa:bb')).toBe(false)
    expect(OCI_RE.region.test('us-phoenix-1')).toBe(true)
    expect(OCI_RE.subnet.test('ocid1.subnet.oc1..x')).toBe(true)
  })
})
