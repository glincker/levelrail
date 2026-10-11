import { describe, expect, it } from 'vitest'
import {
  buildTargetRequest,
  presetById,
  resolveEndpoint,
  type TargetFormValues,
} from './targetPresets'

const base: TargetFormValues = {
  name: 'primary',
  bucket: 'backups',
  region: '',
  endpoint: '',
  accessKeyId: 'AKIA',
  secretAccessKey: 'secret',
}

describe('resolveEndpoint', () => {
  it('fills the region into a provider pattern', () => {
    expect(resolveEndpoint(presetById('wasabi'), '', 'eu-central-1')).toBe(
      'https://s3.eu-central-1.wasabisys.com',
    )
  })

  it('prefers what the operator typed', () => {
    expect(
      resolveEndpoint(presetById('b2'), ' https://s3.custom.example ', 'x'),
    ).toBe('https://s3.custom.example')
  })

  it('leaves R2 empty until the account endpoint is typed', () => {
    expect(resolveEndpoint(presetById('r2'), '', '')).toBe('')
  })
})

describe('buildTargetRequest', () => {
  it('sends no endpoint for AWS', () => {
    const out = buildTargetRequest(presetById('aws'), base)
    expect('request' in out && out.request.endpoint).toBeUndefined()
    expect('request' in out && out.request.provider).toBe('aws')
  })

  it('uses the custom provider with a resolved endpoint for Backblaze B2', () => {
    const out = buildTargetRequest(presetById('b2'), base)
    if (!('request' in out)) throw new Error('expected a request')
    expect(out.request.provider).toBe('custom')
    expect(out.request.endpoint).toBe('https://s3.us-west-004.backblazeb2.com')
    expect(out.request.region).toBe('us-west-004')
  })

  it('reports the first missing field', () => {
    expect(buildTargetRequest(presetById('r2'), base)).toEqual({
      missing: 'endpoint',
    })
    expect(
      buildTargetRequest(presetById('aws'), { ...base, secretAccessKey: '' }),
    ).toEqual({ missing: 'secretAccessKey' })
  })

  it('marks only local disk as living on the same machine', () => {
    expect(presetById('local').sameMachine).toBe(true)
    expect(presetById('minio').sameMachine).toBe(false)
  })
})
