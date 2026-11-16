// Highlighted code splits one command into token spans, so match the <code> textContent instead.
export function commandText(
  pattern: string | RegExp,
): (content: string, element: Element | null) => boolean {
  return (_content, element) => {
    if (!element || element.tagName !== 'CODE') return false
    const text = element.textContent ?? ''
    return typeof pattern === 'string'
      ? text.includes(pattern)
      : pattern.test(text)
  }
}
