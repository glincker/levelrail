import { describe, expect, it } from 'vitest'
import { buildFetch, buildHtml, type FormSchema } from './formSnippet'

const schema: FormSchema = {
  fields: [
    { name: 'email', type: 'email', label: 'Email', required: true },
    { name: 'plan', type: 'select', options: ['free', 'pro'] },
    { name: 'ref', type: 'hidden', default: 'a"b' },
    { name: 'note', type: 'textarea', max_length: 40, placeholder: 'Hi' },
  ],
  success_message: 'Got </script> it',
}

describe('form snippet generator', () => {
  it('falls back to the default three fields', () => {
    const html = buildHtml('https://x.test/api/v1/forms/a/b', '', undefined)
    expect(html).toContain('name="name"')
    expect(html).toContain('type="email" name="email" required')
    expect(html).toContain('<textarea name="message">')
    expect(html).toContain('name="bot-field"')
  })

  it('renders every field type from the schema and escapes values', () => {
    const html = buildHtml('https://x.test/e', 'hp', schema)
    expect(html).toContain('<option value="pro">pro</option>')
    expect(html).toContain('value="a&#34;b"')
    expect(html).toContain('maxlength="40"')
    expect(html).toContain('placeholder="Hi"')
    expect(html).toContain('name="hp"')
  })

  it('minimal markup has no labels and uses placeholders', () => {
    const html = buildHtml('https://x.test/e', 'hp', schema, false)
    expect(html).not.toContain('<label>Email')
    expect(html).toContain('placeholder="Email"')
  })

  it('fetch variant posts JSON and cannot break out of the script tag', () => {
    const js = buildFetch('https://x.test/e', 'hp', schema)
    expect(js).toContain('id="lr-form"')
    expect(js).toContain('"Content-Type": "application/json"')
    expect(js).not.toContain('Got </script>')
    expect(js).toContain('\\u003c/script>')
  })
})
