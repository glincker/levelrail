import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { Brand } from '../../types/brand'
import { NewAppI18n } from '../../test/newAppI18n'
import { UploadSourceCard } from './UploadSourceCard'
import { latestUpload, uploadFileCount } from './uploadSource'

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

const attempt = (over: Partial<DeployAttempt>): DeployAttempt =>
  ({
    id: 'dep_1',
    service_name: 'web',
    image: 'web:abc',
    status: 'succeeded',
    ...over,
  }) as DeployAttempt

describe('uploadFileCount', () => {
  it.each([
    ['Uploaded from CLI, 12 files, abcdef123456', 12],
    ['Uploaded from CLI, 1 file, abcdef123456', 1],
    ['fix typo', null],
    [undefined, null],
  ])('%s', (message, want) => {
    expect(uploadFileCount(message)).toBe(want)
  })
})

describe('latestUpload', () => {
  it('returns the newest upload attempt only', () => {
    const attempts = [
      attempt({ id: 'a', source: 'manual' }),
      attempt({ id: 'b', source: 'upload' }),
      attempt({ id: 'c', source: 'upload' }),
    ]
    expect(latestUpload(attempts)?.id).toBe('b')
    expect(latestUpload([attempt({ source: 'webhook' })])).toBeUndefined()
  })
})

describe('UploadSourceCard', () => {
  it('shows the CLI upload source, hash, file count and a copyable deploy command', () => {
    render(
      <NewAppI18n>
        <UploadSourceCard
          appName="web"
          attempt={attempt({
            source: 'upload',
            commit_sha: 'abcdef1234567890abcdef1234567890abcdef12',
            commit_message: 'Uploaded from CLI, 7 files, abcdef123456',
          })}
        />
      </NewAppI18n>,
    )
    expect(screen.getByText('Source: CLI upload')).toBeInTheDocument()
    expect(screen.getByText('abcdef123456')).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(
      screen.getByText(
        `testbrand-cli deploy --app web --api-url ${window.location.origin}`,
      ),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument()
  })
})
