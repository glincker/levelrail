import { describe, expect, it } from 'vitest'
import { stackTemplateCommands } from './stackTemplateCommands'

describe('stackTemplateCommands', () => {
  it('builds init then apply with secret names', () => {
    const out = stackTemplateCommands(
      'levelrail',
      'web-app',
      { name: 'shop', domain: 'shop.example.com', image: 'nginx:1.27' },
      ['DB_URL'],
    )
    const [init, cd, apply] = out.split('\n')
    expect(init).toBe(
      'levelrail-cli stack init web-app --set name=shop --set domain=shop.example.com --set image=nginx:1.27',
    )
    expect(cd).toBe('cd shop')
    expect(apply).toBe(
      'levelrail-cli stack apply shop -f . --secret DB_URL=DB_URL',
    )
  })

  it('quotes values with spaces', () => {
    const out = stackTemplateCommands('x', 'a', { name: "o'k z" }, [])
    expect(out).toContain(`--set name='o'\\''k z'`)
  })
})
