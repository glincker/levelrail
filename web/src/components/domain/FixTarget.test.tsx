import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { FixTarget } from './FixTarget'
import { validateDomainSearch } from './domainSearch'

describe('validateDomainSearch', () => {
  it.each([
    [{ app: 'web', fix: 'renew' }, 'renew'],
    [{ app: 'web', fix: 'dns' }, 'dns'],
    [{ app: 'web', fix: 'delete' }, undefined],
    [{ app: 'web', fix: 7 }, undefined],
    [{ app: 'web' }, undefined],
  ])('reads fix from %j', (search, want) => {
    expect(validateDomainSearch(search).fix).toBe(want)
  })

  it('keeps app and tab alongside fix', () => {
    expect(
      validateDomainSearch({ app: 'web', tab: 'overview', fix: 'dns' }),
    ).toEqual({ app: 'web', tab: 'overview', fix: 'dns' })
  })
})

describe('FixTarget', () => {
  it('focuses and highlights the control without clicking it', () => {
    const onClick = vi.fn()
    render(
      <FixTarget active>
        <button type="button" onClick={onClick}>
          Renew now
        </button>
      </FixTarget>,
    )
    const button = screen.getByRole('button', { name: 'Renew now' })
    expect(document.activeElement).toBe(button)
    expect(button.parentElement?.getAttribute('data-highlight')).toBe('true')
    expect(onClick).not.toHaveBeenCalled()
  })

  it('leaves focus alone when not the target', () => {
    render(
      <FixTarget active={false}>
        <button type="button">Renew now</button>
      </FixTarget>,
    )
    expect(document.activeElement).not.toBe(
      screen.getByRole('button', { name: 'Renew now' }),
    )
  })
})
