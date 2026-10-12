import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { SettingRow, SettingRows } from './setting-row'

describe('SettingRow', () => {
  it('exposes a named group with its description, badge and control', () => {
    render(
      <SettingRow
        title="Instance URL"
        description="Where this dashboard is served."
        badge={<span>Live</span>}
      >
        <input aria-label="Instance URL value" />
      </SettingRow>,
    )
    const group = screen.getByRole('group', { name: 'Instance URL' })
    expect(group).toHaveAccessibleDescription('Where this dashboard is served.')
    expect(within(group).getByText('Live')).toBeInTheDocument()
    expect(within(group).getByRole('textbox')).toBeInTheDocument()
  })

  it('renders the label column before the control column', () => {
    render(
      <SettingRow title="Title">
        <button type="button">Control</button>
      </SettingRow>,
    )
    const group = screen.getByRole('group', { name: 'Title' })
    const [label, control] = Array.from(group.children)
    expect(label?.textContent).toBe('Title')
    expect(control?.querySelector('button')).not.toBeNull()
  })

  it('keeps rows in source order inside SettingRows', () => {
    render(
      <SettingRows>
        <SettingRow title="First">
          <span>a</span>
        </SettingRow>
        <SettingRow title="Second">
          <span>b</span>
        </SettingRow>
      </SettingRows>,
    )
    const groups = screen.getAllByRole('group')
    expect(groups).toHaveLength(2)
    expect(groups[0]).toHaveAccessibleName('First')
    expect(groups[1]).toHaveAccessibleName('Second')
  })
})
