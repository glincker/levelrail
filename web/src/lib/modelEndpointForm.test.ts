import { describe, expect, it } from 'vitest'
import {
  endpointFormErrors,
  inferApiType,
  isValidEndpointUrl,
  suggestEndpointName,
} from './modelEndpointForm'

describe('modelEndpointForm', () => {
  it.each([
    ['http://100.64.0.7:11434', 'ollama'],
    ['http://mac-mini.local:1234/v1', 'openai'],
    ['not a url', 'openai'],
  ] as const)('infers the api type of %s', (url, want) => {
    expect(inferApiType(url)).toBe(want)
  })

  it.each([
    ['http://Mac-Mini.local:11434', 'mac-mini'],
    ['http://100.64.0.7:11434', 'ep-100-64-0-7'],
    ['https://llm.example.com/v1', 'llm-example-com'],
    ['ftp://x', ''],
    ['', ''],
  ])('suggests a name for %s', (url, want) => {
    expect(suggestEndpointName(url)).toBe(want)
  })

  it('rejects credentials and non http urls', () => {
    expect(isValidEndpointUrl('http://user:pw@10.0.0.1')).toBe(false)
    expect(isValidEndpointUrl('file:///etc/passwd')).toBe(false)
    expect(isValidEndpointUrl('http://10.0.0.1:11434')).toBe(true)
  })

  it('accepts an empty name when one can be inferred', () => {
    expect(
      endpointFormErrors({ name: '', url: 'http://mac.local:11434' }),
    ).toEqual({})
    expect(
      endpointFormErrors({ name: 'Bad Name', url: 'http://mac.local' }),
    ).toEqual({
      name: true,
    })
    expect(endpointFormErrors({ name: 'mac', url: '' })).toEqual({ url: true })
  })
})
