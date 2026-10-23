import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { CommonI18n } from '../../test/commonI18n'
import { SettingsSectionNav } from './SettingsSectionNav'
import { SettingsGroup } from './SettingsGroup'
import { CardHelp } from './CardHelp'
import { DeployFlowStrip } from './DeployFlowStrip'

function renderAll() {
  render(
    <CommonI18n>
      <DeployFlowStrip />
      <SettingsSectionNav />
      <SettingsGroup id="build">
        <CardHelp card="buildCache">
          <div>cache card</div>
        </CardHelp>
      </SettingsGroup>
      <SettingsGroup id="safety">
        <CardHelp card="safety" recommended>
          <div>safety card</div>
        </CardHelp>
      </SettingsGroup>
    </CommonI18n>,
  )
}

describe('deploy settings layout', () => {
  it('links each section nav item to its group anchor', () => {
    renderAll()
    const nav = screen.getByRole('navigation', {
      name: 'Deploy settings sections',
    })
    const hrefs = within(nav)
      .getAllByRole('link')
      .map((a) => a.getAttribute('href'))
    expect(hrefs).toEqual(['#build', '#run', '#safety', '#automation'])
  })

  it('renders a described group with a heading', () => {
    renderAll()
    expect(
      screen.getByRole('heading', { level: 2, name: 'Build' }),
    ).toBeInTheDocument()
    expect(screen.getByText(/How your source becomes an image/)).toBeVisible()
  })

  it('shows the what-this-does line and recommended hint on a card', () => {
    renderAll()
    expect(screen.getAllByText('What this does:')).toHaveLength(2)
    expect(screen.getByText(/platform default/)).toBeInTheDocument()
  })

  it('lists the four deploy steps as links', () => {
    renderAll()
    const strip = screen.getByRole('region', { name: 'How your deploy works' })
    expect(within(strip).getAllByRole('link')).toHaveLength(4)
  })
})
