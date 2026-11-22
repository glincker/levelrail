import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { AppsGroupHeader } from './AppsGroupHeader'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, o?: { count?: number }) =>
      o?.count === undefined ? key : `${key}:${o.count}`,
  }),
}))

describe('AppsGroupHeader', () => {
  it('shows the count and selects or collapses the group', async () => {
    const onSelectGroup = vi.fn()
    const onToggle = vi.fn()
    render(
      <AppsGroupHeader
        label="node-1"
        count={3}
        selectedCount={0}
        collapsed={false}
        onToggle={onToggle}
        onSelectGroup={onSelectGroup}
      />,
    )
    expect(screen.getByText('group.count:3')).toBeTruthy()
    await userEvent.click(screen.getByText('group.select'))
    expect(onSelectGroup).toHaveBeenCalledWith(true)
    await userEvent.click(screen.getByRole('button', { name: /node-1/ }))
    expect(onToggle).toHaveBeenCalled()
  })
})
