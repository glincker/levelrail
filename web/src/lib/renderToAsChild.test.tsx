import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderToAsChild } from './renderToAsChild'

describe('renderToAsChild', () => {
  it('passes children through when there is no render prop', () => {
    expect(renderToAsChild(undefined, 'x')).toEqual({ children: 'x' })
  })

  it('wraps children in the render element and sets asChild', () => {
    const out = renderToAsChild(<a href="/x" />, 'go')
    expect(out.asChild).toBe(true)
    render(<>{out.children}</>)
    expect(screen.getByRole('link', { name: 'go' })).toHaveAttribute(
      'href',
      '/x',
    )
  })

  it('accepts a render function', () => {
    const out = renderToAsChild(() => <button type="button" />, 'ok')
    render(<>{out.children}</>)
    expect(screen.getByRole('button', { name: 'ok' })).toBeInTheDocument()
  })
})
