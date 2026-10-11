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
})
