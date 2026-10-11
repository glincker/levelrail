import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { DnsRecordSet } from '../../types/dns'
import { RecordsTable } from './RecordsTable'
import { renderDns } from './dnsTestUtils'

const records: DnsRecordSet[] = [
  { name: '@', type: 'NS', ttl: 86400, values: ['ns1.x.net'], managed: true },
  { name: 'www', type: 'A', ttl: 300, values: ['203.0.113.1'], proxied: true },
  { name: '@', type: 'MX', ttl: 3600, values: ['10 mail.example.com'] },
  {
    name: 'lb',
    type: 'A',
    ttl: 60,
    values: ['198.51.100.1'],
    routing: 'weighted',
    set_identifier: 'blue',
    weight: 10,
  },
]

function setup(list = records) {
  const handlers = { onAdd: vi.fn(), onEdit: vi.fn(), onDelete: vi.fn() }
  renderDns(<RecordsTable records={list} {...handlers} />)
  return handlers
}

describe('RecordsTable', () => {
  it('renders every set with flags and hides actions on managed sets', () => {
    setup()
    expect(screen.getByText('4 record sets')).toBeInTheDocument()
    expect(screen.getByText('Proxied')).toBeInTheDocument()
    expect(screen.getByText('weighted blue')).toBeInTheDocument()
    expect(screen.getByText('Managed')).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Edit @ NS' }),
    ).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Edit www A' })).toBeEnabled()
  })

  it('searches names and values', async () => {
    setup()
    await userEvent.type(
      screen.getByRole('searchbox', { name: 'Search name or value' }),
      'mail',
    )
    expect(screen.getByText('1 record set')).toBeInTheDocument()
    expect(screen.getByText('10 mail.example.com')).toBeInTheDocument()
    await userEvent.clear(screen.getByRole('searchbox'))
    await userEvent.type(screen.getByRole('searchbox'), 'nothing-here')
    expect(screen.getByText('No records match')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(screen.getByText('4 record sets')).toBeInTheDocument()
  })

  it('calls edit and delete with the clicked set', async () => {
    const h = setup()
    await userEvent.click(screen.getByRole('button', { name: 'Edit www A' }))
    expect(h.onEdit).toHaveBeenCalledWith(records[1])
    await userEvent.click(screen.getByRole('button', { name: 'Delete lb A' }))
    expect(h.onDelete).toHaveBeenCalledWith(records[3])
  })

  it('shows an add action when the zone is empty', async () => {
    const h = setup([])
    expect(screen.getByText('No records yet')).toBeInTheDocument()
    await userEvent.click(
      screen.getAllByRole('button', { name: 'Add record' })[1]!,
    )
    expect(h.onAdd).toHaveBeenCalled()
  })

  it('virtualizes beyond 50 rows instead of rendering them all', () => {
    const many = Array.from({ length: 120 }, (_, i) => ({
      name: `host${i}`,
      type: 'A',
      ttl: 300,
      values: [`10.0.0.${i % 250}`],
    }))
    setup(many)
    expect(screen.getByText('120 record sets')).toBeInTheDocument()
    expect(screen.queryAllByText(/^host\d+$/).length).toBeLessThan(120)
  })
})
