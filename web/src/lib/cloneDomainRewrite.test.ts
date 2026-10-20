import { describe, expect, it } from 'vitest'
import {
  EMPTY_DOMAIN_REWRITE,
  domainRewritePayload,
  type CloneDomainRewrite,
} from './cloneDomainRewrite'

const base: CloneDomainRewrite = { ...EMPTY_DOMAIN_REWRITE, copyDomains: true }

describe('domainRewritePayload', () => {
  it.each([
    ['off', EMPTY_DOMAIN_REWRITE, null],
    [
      'prefix mode with prefix',
      { ...base, prefix: ' uat. ' },
      { prefix: 'uat.' },
    ],
    ['prefix mode empty', base, null],
    [
      'replace mode complete',
      {
        ...base,
        mode: 'replace' as const,
        find: 'a.com',
        replace: 'uat.a.com',
      },
      { find: 'a.com', replace: 'uat.a.com' },
    ],
    [
      'replace mode missing target',
      { ...base, mode: 'replace' as const, find: 'a.com' },
      null,
    ],
  ])('%s', (_name, input, want) => {
    expect(domainRewritePayload(input)).toEqual(want)
  })
})
