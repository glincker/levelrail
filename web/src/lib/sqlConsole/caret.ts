const COPIED = [
  'boxSizing',
  'width',
  'fontFamily',
  'fontSize',
  'fontWeight',
  'lineHeight',
  'letterSpacing',
  'paddingTop',
  'paddingRight',
  'paddingBottom',
  'paddingLeft',
  'borderTopWidth',
  'borderRightWidth',
  'borderBottomWidth',
  'borderLeftWidth',
  'tabSize',
  'textIndent',
] as const

// Caret position inside a textarea, relative to its box, measured with a
// hidden mirror that wraps text the same way.
export function caretPosition(
  el: HTMLTextAreaElement,
  index: number,
): { top: number; left: number; height: number } {
  const mirror = document.createElement('div')
  const cs = window.getComputedStyle(el)
  for (const p of COPIED) {
    mirror.style[p] = cs[p]
  }
  mirror.style.position = 'absolute'
  mirror.style.visibility = 'hidden'
  mirror.style.whiteSpace = 'pre-wrap'
  mirror.style.overflowWrap = 'break-word'
  mirror.textContent = el.value.slice(0, index)
  const marker = document.createElement('span')
  marker.textContent = '​'
  mirror.appendChild(marker)
  document.body.appendChild(mirror)
  const height = parseFloat(cs.lineHeight) || marker.offsetHeight
  const pos = {
    top: marker.offsetTop - el.scrollTop,
    left: marker.offsetLeft - el.scrollLeft,
    height,
  }
  mirror.remove()
  return pos
}
