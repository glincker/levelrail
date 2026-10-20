import { describe, expect, it } from 'vitest'
import { chatCurlExample, parseModelTab } from './modelPresentation'

describe('parseModelTab', () => {
  it.each([
    ['keys', 'keys'],
    ['logs', 'logs'],
    ['nope', 'overview'],
    [undefined, 'overview'],
  ])('maps %s to %s', (input, want) => {
    expect(parseModelTab(input)).toBe(want)
  })
})

describe('chatCurlExample', () => {
  it('targets chat completions and keeps the key out of the text', () => {
    const out = chatCurlExample('https://chat.example.com/v1/', 'llama3.1:8b')
    expect(out).toContain('https://chat.example.com/v1/chat/completions')
    expect(out).toContain('$API_KEY')
    expect(out).toContain('"model":"llama3.1:8b"')
  })
})
