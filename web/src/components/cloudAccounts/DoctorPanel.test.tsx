import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { DoctorPanel } from './DoctorPanel'

const apiRequest = vi.fn<(path: string) => Promise<unknown>>()

vi.mock('../../lib/apiRequest', () => ({
  apiRequest: (path: string) => apiRequest(path),
}))

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

beforeEach(() => {
  apiRequest.mockReset()
  apiRequest.mockImplementation((path: string) =>
    Promise.resolve(
      path.includes('deep=1')
        ? {
            ready: true,
            steps: [
              {
                id: 'deep.aws.ec2',
                title: 'EC2 instances',
                status: 'warn',
                detail: 'denied',
                fix: 'Allow ec2:DescribeInstances',
              },
            ],
          }
        : { ready: true, steps: [] },
    ),
  )
})

describe('DoctorPanel deep check', () => {
  it('runs the deep request and lists its steps', async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <DoctorPanel id="acc1" />
      </QueryClientProvider>,
    )
    await userEvent.click(await screen.findByText('doctor.deep'))
    await waitFor(() => expect(screen.getByText('EC2 instances')).toBeTruthy())
    expect(apiRequest).toHaveBeenCalledWith(
      '/api/v1/cloud-accounts/acc1/doctor?deep=1',
    )
  })
})
