import { describe, expect, it } from 'vitest'
import {
  formatBytes,
  parseHfRef,
  tokenFingerprint,
  withQuant,
} from './modelPreflight'

describe('parseHfRef', () => {
  it.each([
    ['llamacpp', 'acme/chat-GGUF', 'acme/chat-GGUF', '', ''],
    ['llamacpp', 'acme/chat-GGUF:Q4_K_M', 'acme/chat-GGUF', 'Q4_K_M', ''],
    [
      'vllm',
      'meta-llama/Llama-3.1-8B-Instruct',
      'meta-llama/Llama-3.1-8B-Instruct',
      '',
      '',
    ],
    ['vllm', 'https://huggingface.co/acme/chat', 'acme/chat', '', ''],
    ['ollama', 'hf.co/acme/chat-GGUF:Q8_0', 'acme/chat-GGUF', 'Q8_0', 'hf.co/'],
  ] as const)('%s %s', (engine, input, repo, quant, prefix) => {
    expect(parseHfRef(engine, input)).toEqual({ repo, quant, prefix })
  })

  it.each([
    ['ollama', 'llama3.1:8b'],
    ['ollama', 'acme/chat'],
    ['llamacpp', 'llama3.1:8b'],
    ['llamacpp', ''],
    ['vllm', 'a/b/c'],
    ['vllm', 'acme/has space'],
  ] as const)('%s %s is not a Hub repo', (engine, input) => {
    expect(parseHfRef(engine, input)).toBeNull()
  })

  it('keeps the hf.co prefix when swapping the quant for Ollama', () => {
    const ref = parseHfRef('ollama', 'hf.co/acme/chat-GGUF')
    expect(ref && withQuant(ref, 'Q4_K_M')).toBe('hf.co/acme/chat-GGUF:Q4_K_M')
    const plain = parseHfRef('llamacpp', 'acme/chat-GGUF')
    expect(plain && withQuant(plain, 'Q4_K_M')).toBe('acme/chat-GGUF:Q4_K_M')
  })
})

describe('formatBytes', () => {
  it.each([
    [512, '512 B'],
    [2048, '2.0 KiB'],
    [5 * 1024 ** 3, '5.0 GiB'],
    [150 * 1024 ** 3, '150 GiB'],
  ])('%d', (bytes, want) => {
    expect(formatBytes(bytes)).toBe(want)
  })
})

describe('tokenFingerprint', () => {
  it('is empty for no token and never contains the token', () => {
    expect(tokenFingerprint('')).toBe('')
    const fp = tokenFingerprint('hf_secret_value')
    expect(fp).not.toContain('secret')
    expect(fp).not.toBe(tokenFingerprint('hf_other_value'))
  })
})
