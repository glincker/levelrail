import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Table, TableBody, TableCell, TableRow } from './table'

describe('Table', () => {
  it('scrolls horizontally and pins the first column below md', () => {
    const { container } = render(
      <Table>
        <TableBody>
          <TableRow>
            <TableCell>name</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    )
    const wrapper = container.querySelector('[data-slot="table-container"]')
    expect(wrapper?.className).toContain('overflow-x-auto')
    const table = container.querySelector('table')
    expect(table?.className).toContain('max-md:[&_tr>:first-child]:sticky')
  })

  it('marks the container scrolled and keeps the selected row state', () => {
    const { container } = render(
      <Table>
        <TableBody>
          <TableRow data-state="selected">
            <TableCell>name</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    )
    const wrapper = container.querySelector('[data-slot="table-container"]')
    expect(wrapper?.hasAttribute('data-scrolled')).toBe(false)
    expect(container.querySelector('tr')?.getAttribute('data-state')).toBe(
      'selected',
    )
  })

  it('defaults to balanced density and accepts compact and spacious', () => {
    const { container, rerender } = render(<Table />)
    expect(container.querySelector('table')?.dataset.density).toBe('balanced')
    rerender(<Table density="compact" />)
    expect(container.querySelector('table')?.dataset.density).toBe('compact')
  })
})
