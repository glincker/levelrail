import { isValidElement } from 'react'
import { Link } from '@tanstack/react-router'

/**
 * Base UI button primitives assume a native <button> unless told otherwise.
 * Derives the nativeButton prop from the element passed through `render`:
 * a non-button tag or a router Link is not native, a <button> is, anything
 * else is unknown and left to Base UI's default.
 */
export function nativeButtonFromRender(
  render: unknown,
  explicit?: boolean,
): boolean | undefined {
  if (explicit !== undefined) return explicit
  if (!isValidElement(render)) return undefined
  const type = render.type
  if (typeof type === 'string') return type === 'button'
  if (type === Link) return false
  return undefined
}
