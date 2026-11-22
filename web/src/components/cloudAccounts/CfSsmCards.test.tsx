import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CloudFrontCard } from './CloudFrontCard'
import { SsmCard } from './SsmCard'

const invalidate = vi.fn()
const run = vi.fn()
const idle = {
  mutate: vi.fn(),
  reset: vi.fn(),
  isPending: false,
  isError: false,
  isSuccess: false,
  error: null,
  data: undefined,
}

vi.mock('../../queries/cloudCfSsm', () => ({
  useAwsCfDistributions: () => ({
    isPending: false,
    error: null,
    data: [
      {
        id: 'E1',
        status: 'Deployed',
        domain: 'd.net',
        comment: '',
        enabled: true,
        aliases: [],
      },
    ],
  }),
  useAwsCfInvalidations: () => ({ isPending: false, error: null, data: [] }),
  useAwsCfInvalidate: () => ({ ...idle, mutate: invalidate }),
  useAwsSsmInstances: () => ({
    isPending: false,
    error: null,
    data: {
      region: 'us-east-1',
      instances: [
        {
          id: 'i-0abc',
          ping_status: 'Online',
          platform: 'Linux',
          platform_name: 'Ubuntu',
          agent_version: '3',
          computer_name: 'web',
          last_ping: '',
        },
      ],
    },
  }),
  useAwsSsmRun: () => ({ ...idle, mutate: run }),
  useAwsSsmRefresh: () => idle,
  useAwsSsmSession: () => idle,
}))

beforeEach(() => {
  invalidate.mockReset()
  run.mockReset()
})

describe('CloudFrontCard', () => {
  it('sends one trimmed path per line', async () => {
    render(<CloudFrontCard id="a" />)
    await userEvent.click(
      screen.getByRole('button', { name: 'services.cloudfront.select' }),
    )
    await userEvent.type(screen.getByRole('textbox'), '/a{enter} /b ')
    await userEvent.click(
      screen.getByRole('button', { name: 'services.cloudfront.invalidate' }),
    )
    expect(invalidate.mock.calls[0]?.[0]).toEqual({
      did: 'E1',
      paths: ['/a', '/b'],
    })
  })
})

describe('SsmCard', () => {
  it('shows Run Command only for root with the flag on', () => {
    const { rerender } = render(<SsmCard id="a" isRoot={false} runEnabled />)
    expect(screen.getByText('i-0abc')).toBeInTheDocument()
    expect(screen.queryByText('services.ssm.runTitle')).toBeNull()
    rerender(<SsmCard id="a" isRoot runEnabled={false} />)
    expect(screen.queryByText('services.ssm.runTitle')).toBeNull()
    rerender(<SsmCard id="a" isRoot runEnabled />)
    expect(screen.getByText('services.ssm.runTitle')).toBeInTheDocument()
  })
})
