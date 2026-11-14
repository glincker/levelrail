const SIZE_PROPS = {
  offsetWidth: 'width',
  clientWidth: 'width',
  offsetHeight: 'height',
  clientHeight: 'height',
} as const

// jsdom has no layout. The virtualizer reads offset* or client* sizes
// depending on its version, so stub both and return a restore function.
export function stubLayoutSize(width: number, height: number): () => void {
  const saved = new Map<string, PropertyDescriptor | undefined>()
  for (const [prop, axis] of Object.entries(SIZE_PROPS)) {
    saved.set(
      prop,
      Object.getOwnPropertyDescriptor(HTMLElement.prototype, prop),
    )
    Object.defineProperty(HTMLElement.prototype, prop, {
      configurable: true,
      get: () => (axis === 'width' ? width : height),
    })
  }
  return () => {
    for (const [prop, desc] of saved) {
      if (desc) Object.defineProperty(HTMLElement.prototype, prop, desc)
      else
        delete (HTMLElement.prototype as unknown as Record<string, unknown>)[
          prop
        ]
    }
  }
}
