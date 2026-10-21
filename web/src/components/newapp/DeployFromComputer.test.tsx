import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { Brand } from '../../types/brand'
import { NewAppI18n } from '../../test/newAppI18n'
import { DeployFromComputer } from './DeployFromComputer'

vi.mock('../../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: '',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: '',
    DiscussionsURL: '',
    RepoURL: '',
  }),
}))

describe('DeployFromComputer', () => {
  it('shows the exact command with this control plane URL filled in', () => {
    render(
      <NewAppI18n>
        <DeployFromComputer />
      </NewAppI18n>,
    )
    expect(
      screen.getByRole('heading', { name: 'Deploy from your computer' }),
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        `testbrand-cli deploy --api-url ${window.location.origin}`,
      ),
    ).toBeInTheDocument()
  })
})
