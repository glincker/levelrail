import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { RedisValuePanel } from './RedisValuePanel'
import { DatabasesI18n } from '../../test/databasesI18n'
import type { RedisValue } from '../../types/databaseViewer'

const base: RedisValue = {
  type: 'string',
  ttl: -1,
  lines: ['{"a":1}'],
  truncated: false,
  encoding: 'embstr',
  size: 7,
  memoryBytes: 64,
  entries: [],
}

function show(v: RedisValue) {
  render(
    <DatabasesI18n>
      <RedisValuePanel value={v} />
    </DatabasesI18n>,
  )
}

describe('RedisValuePanel', () => {
  it('shows badges and toggles string views', () => {
    show(base)
    expect(screen.getByText(/embstr/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Hex' }))
    expect(screen.getByText(/7b 22 61/)).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'JSON' }))
    expect(screen.getByText(/\{1\}/)).toBeTruthy()
  })

  it('renders stream entries and the JSON unsupported marker', () => {
    show({
      ...base,
      type: 'stream',
      entries: [{ id: '1-0', pairs: [{ field: 'name', value: 'ada' }] }],
    })
    expect(screen.getByText('1-0')).toBeTruthy()
    show({ ...base, type: 'ReJSON-RL', unsupported: true })
    expect(screen.getByText(/JSON module is not available/)).toBeTruthy()
  })
})
