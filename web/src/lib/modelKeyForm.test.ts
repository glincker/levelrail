import { describe, expect, it } from 'vitest'
import { buildKeyRequest, curlExample } from './modelKeyForm'

const EMPTY = {
  name: '',
  expiresDays: '',
  rpm: '',
  tpm: '',
  tpd: '',
  maxParallel: '',
  allowPaths: '',
  allowModels: '',
}

describe('buildKeyRequest', () => {
  it('leaves empty limits unset and trims the name', () => {
    const req = buildKeyRequest({ ...EMPTY, name: '  ci ' }, 0)
    expect(req.name).toBe('ci')
    expect(req.rpm).toBeUndefined()
    expect(req.expires_at).toBeUndefined()
    expect(req.allow_paths).toEqual([])
  })

  it('parses limits, lists and expiry', () => {
    const req = buildKeyRequest(
      {
        ...EMPTY,
        name: 'ci',
        rpm: '30',
        tpm: 'x',
        tpd: '5000',
        maxParallel: '2',
        expiresDays: '1',
        allowPaths: '/v1/chat/completions, /v1/embeddings ,',
      },
      0,
    )
    expect(req.rpm).toBe(30)
    expect(req.tpm).toBeUndefined()
    expect(req.tpd).toBe(5000)
    expect(req.max_parallel).toBe(2)
    expect(req.expires_at).toBe('1970-01-02T00:00:00.000Z')
    expect(req.allow_paths).toEqual(['/v1/chat/completions', '/v1/embeddings'])
  })
})

describe('curlExample', () => {
  it('targets the base url without a doubled slash', () => {
    expect(curlExample('https://m.example.com/v1/', 'lr-abc')).toBe(
      'curl https://m.example.com/v1/models -H "Authorization: Bearer lr-abc"',
    )
  })
})
