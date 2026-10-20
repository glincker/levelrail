// Static accessibility checks for rendered DOM, covering the rules axe-core
// flags most often in this codebase: unnamed controls, unlabelled form
// fields, images without alt text and duplicate ids.

function hiddenFromAt(el: Element): boolean {
  for (let node: Element | null = el; node; node = node.parentElement) {
    if (node.getAttribute('aria-hidden') === 'true') return true
    if (node.hasAttribute('hidden')) return true
  }
  return false
}

function textOf(el: Element): string {
  return (el.textContent ?? '').trim()
}

function labelledByText(el: Element, root: ParentNode): string {
  const ids = (el.getAttribute('aria-labelledby') ?? '').split(/\s+/)
  return ids
    .map((id) =>
      id ? (root.querySelector(`[id="${id}"]`)?.textContent ?? '') : '',
    )
    .join('')
    .trim()
}

function hasName(el: Element, root: ParentNode): boolean {
  if ((el.getAttribute('aria-label') ?? '').trim() !== '') return true
  if (labelledByText(el, root) !== '') return true
  if ((el.getAttribute('title') ?? '').trim() !== '') return true
  if (textOf(el) !== '') return true
  const img = el.querySelector('img[alt]')
  return img !== null && (img.getAttribute('alt') ?? '').trim() !== ''
}

function fieldHasLabel(el: Element, root: ParentNode): boolean {
  if ((el.getAttribute('aria-label') ?? '').trim() !== '') return true
  if (labelledByText(el, root) !== '') return true
  if ((el.getAttribute('title') ?? '').trim() !== '') return true
  if (el.closest('label')) return true
  const id = el.getAttribute('id')
  return id !== null && root.querySelector(`label[for="${id}"]`) !== null
}

export function a11yViolations(root: ParentNode = document.body): string[] {
  const out: string[] = []

  for (const img of root.querySelectorAll('img')) {
    if (hiddenFromAt(img) || img.getAttribute('role') === 'presentation')
      continue
    if (!img.hasAttribute('alt'))
      out.push(`img without alt: ${img.outerHTML.slice(0, 80)}`)
  }

  for (const el of root.querySelectorAll('button, [role="button"]')) {
    if (hiddenFromAt(el)) continue
    if (!hasName(el, root))
      out.push(
        `button without an accessible name: ${el.outerHTML.slice(0, 100)}`,
      )
  }

  for (const el of root.querySelectorAll('a[href]')) {
    if (hiddenFromAt(el)) continue
    if (!hasName(el, root))
      out.push(`link without an accessible name: ${el.outerHTML.slice(0, 100)}`)
  }

  for (const el of root.querySelectorAll('input, select, textarea')) {
    if (hiddenFromAt(el)) continue
    const type = el.getAttribute('type')
    if (type === 'hidden' || type === 'submit' || type === 'button') continue
    if (!fieldHasLabel(el, root))
      out.push(`form field without a label: ${el.outerHTML.slice(0, 100)}`)
  }

  const NAMED_ROLES =
    'dialog, alertdialog, [role="tab"], [role="switch"], [role="checkbox"], [role="radio"], [role="combobox"], [role="menuitem"], [role="option"], [role="slider"], [role="listbox"], [role="tablist"], [role="searchbox"]'
  for (const el of root.querySelectorAll(NAMED_ROLES)) {
    if (hiddenFromAt(el)) continue
    if (!hasName(el, root))
      out.push(`ARIA role without a name: ${el.outerHTML.slice(0, 100)}`)
  }

  for (const el of root.querySelectorAll('[tabindex]')) {
    if (Number(el.getAttribute('tabindex')) > 0)
      out.push(`positive tabindex: ${el.outerHTML.slice(0, 80)}`)
  }

  for (const el of root.querySelectorAll(
    '[aria-hidden="true"] a[href], [aria-hidden="true"] button:not([disabled]), [aria-hidden="true"] input:not([disabled])',
  )) {
    if (el.getAttribute('tabindex') === '-1') continue
    out.push(`focusable inside aria-hidden: ${el.outerHTML.slice(0, 80)}`)
  }

  for (const attr of ['aria-labelledby', 'aria-describedby', 'aria-controls']) {
    for (const el of root.querySelectorAll(`[${attr}]`)) {
      for (const id of (el.getAttribute(attr) ?? '').split(/\s+/)) {
        if (id && !root.querySelector(`[id="${id}"]`))
          out.push(`${attr} points at missing id ${id}`)
      }
    }
  }

  for (const el of root.querySelectorAll(
    'a[href] button, button button, a[href] a[href]',
  )) {
    out.push(`nested interactive control: ${el.outerHTML.slice(0, 80)}`)
  }

  for (const el of root.querySelectorAll('h1, h2, h3, h4, h5, h6')) {
    if (hiddenFromAt(el)) continue
    if (textOf(el) === '' && !el.querySelector('img[alt]'))
      out.push(`empty heading: ${el.outerHTML.slice(0, 80)}`)
  }

  for (const el of root.querySelectorAll('iframe')) {
    if ((el.getAttribute('title') ?? '').trim() === '')
      out.push('iframe without a title')
  }

  const seen = new Set<string>()
  for (const el of root.querySelectorAll('[id]')) {
    const id = el.getAttribute('id') ?? ''
    if (seen.has(id)) out.push(`duplicate id: ${id}`)
    seen.add(id)
  }

  return out
}
