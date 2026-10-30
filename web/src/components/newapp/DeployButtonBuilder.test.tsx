import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { NewAppI18n } from '../../test/newAppI18n'
import DeployButtonBuilder from './DeployButtonBuilder'

function renderBuilder() {
  render(
    <NewAppI18n>
      <DeployButtonBuilder />
    </NewAppI18n>,
  )
}

describe('DeployButtonBuilder', () => {
  it('shows no snippet until the image is valid', async () => {
    renderBuilder()
    const user = userEvent.setup()
    expect(screen.queryByLabelText('Markdown snippet')).toBeNull()
    await user.type(screen.getByLabelText('Image'), 'Not An Image')
    expect(screen.queryByLabelText('Markdown snippet')).toBeNull()
    expect(screen.getByText(/Enter an image such as/)).toBeTruthy()
  })

  it('builds a markdown snippet from the fields', async () => {
    renderBuilder()
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('Image'), 'ghcr.io/you/app:1')
    await user.type(screen.getByLabelText(/Port/), '3000')
    await user.type(
      screen.getByLabelText(/Environment variable names/),
      'API_KEY',
    )
    const snippet = screen.getByLabelText('Markdown snippet').textContent ?? ''
    expect(snippet).toContain('image=ghcr.io%2Fyou%2Fapp%3A1')
    expect(snippet).toContain('port=3000')
    expect(snippet).toContain('env=API_KEY')
    expect(screen.getByRole('button', { name: 'Copy Markdown' })).toBeTruthy()
  })

  it('rejects env entries that carry a value', async () => {
    renderBuilder()
    const user = userEvent.setup()
    await user.type(screen.getByLabelText('Image'), 'nginx')
    await user.type(
      screen.getByLabelText(/Environment variable names/),
      'KEY=secret',
    )
    expect(screen.queryByLabelText('Markdown snippet')).toBeNull()
    expect(screen.getByText(/no values/)).toBeTruthy()
  })
})
