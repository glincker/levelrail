import { afterEach, describe, expect, it } from 'vitest'
import { readLogSelection } from './logSelection'

function select(node: Node) {
  const range = document.createRange()
  range.selectNodeContents(node)
  const sel = window.getSelection()
  sel?.removeAllRanges()
  sel?.addRange(range)
}

describe('readLogSelection', () => {
  afterEach(() => {
    window.getSelection()?.removeAllRanges()
    document.body.innerHTML = ''
  })

  it('returns the selected text inside the container', () => {
    document.body.innerHTML =
      '<div id="c"><p id="a">hello</p></div><p id="o">other</p>'
    select(document.getElementById('a') as Node)
    expect(readLogSelection(document.getElementById('c'))).toBe('hello')
  })

  it('ignores a selection outside the container', () => {
    document.body.innerHTML =
      '<div id="c"><p>hello</p></div><p id="o">other</p>'
    select(document.getElementById('o') as Node)
    expect(readLogSelection(document.getElementById('c'))).toBe('')
  })

  it('returns empty for a collapsed selection or a missing container', () => {
    document.body.innerHTML = '<div id="c">hello</div>'
    expect(readLogSelection(document.getElementById('c'))).toBe('')
    expect(readLogSelection(null)).toBe('')
  })
})
