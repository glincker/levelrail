import { cloneElement, isValidElement } from 'react'
import type { ReactElement, ReactNode } from 'react'

export type RenderProp = ReactElement | ((props: object) => ReactElement)

type AsChildProps = { asChild?: true; children?: ReactNode }

// Base UI composes with `render`, Radix with `asChild`: this lets an adapter keep
// the `render` call sites while the glinui primitive underneath reads `asChild`.
export function renderToAsChild(
  render: RenderProp | undefined,
  children?: ReactNode,
): AsChildProps {
  if (render === undefined) return { children }
  const element = typeof render === 'function' ? render({}) : render
  if (!isValidElement(element)) return { children }
  return { asChild: true, children: cloneElement(element, undefined, children) }
}
