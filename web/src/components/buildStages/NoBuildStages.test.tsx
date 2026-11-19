import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { DeployDetailI18n } from '@/test/deployDetailI18n'
import { NoBuildStages } from './NoBuildStages'

describe('NoBuildStages for a static site', () => {
  afterEach(() => {
    cleanup()
  })

  it('shows Build, Publish and Live, never container steps', () => {
    render(
      <DeployDetailI18n>
        <NoBuildStages
          prebuilt={false}
          steps={[
            { key: 'build', status: 'done' },
            { key: 'publish', status: 'done' },
            { key: 'live', status: 'done' },
          ]}
        />
      </DeployDetailI18n>,
    )
    expect(screen.getByText('Build')).toBeInTheDocument()
    expect(screen.getByText('Publish')).toBeInTheDocument()
    expect(screen.getByText('Live')).toBeInTheDocument()
    expect(screen.queryByText('Pull image')).toBeNull()
    expect(screen.queryByText('Create container')).toBeNull()
  })
})
