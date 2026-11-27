import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  DemoSampleAppResult,
  DemoSampleDatabaseResult,
  DemoSampleDnsCheck,
  DemoSampleEnvSaved,
  DemoSampleLogs,
  DemoSampleScale,
} from './DemoSampleResults'
import {
  DEMO_MASK,
  demoSampleConnection,
  demoSampleUrl,
  diffEnvKeys,
} from '@/lib/demoSamples'

describe('demo sample results', () => {
  it('labels the app result and shows a sample url and build log', () => {
    render(<DemoSampleAppResult name="My App" />)
    expect(screen.getByText('sample.badge')).toBeTruthy()
    expect(screen.getByText(demoSampleUrl('My App'))).toBeTruthy()
    expect(screen.getByLabelText('sample.app.logLabel')).toBeTruthy()
  })

  it('masks the database secret', () => {
    render(<DemoSampleDatabaseResult name="main" engine="postgres" />)
    const conn = demoSampleConnection('postgres', 'main')
    expect(conn).toContain(DEMO_MASK)
    expect(screen.getByLabelText('sample.db.connectionLabel').textContent).toBe(
      conn,
    )
  })

  it('diffs env keys without values', () => {
    const diff = diffEnvKeys({ A: '1', B: '2' }, { A: '9', C: '3' })
    expect(diff).toEqual({ added: ['C'], changed: ['A'], removed: ['B'] })
    render(<DemoSampleEnvSaved diff={diff} />)
    expect(screen.getByText('sample.env.saved')).toBeTruthy()
    expect(screen.queryByText('9')).toBeNull()
  })

  it('previews a scale change with rollout steps', async () => {
    render(<DemoSampleScale replicas={2} />)
    await userEvent.click(
      screen.getByRole('button', { name: 'sample.scale.preview' }),
    )
    expect(screen.getByText('sample.scale.change')).toBeTruthy()
    expect(screen.getByText('sample.scale.steps.done')).toBeTruthy()
  })

  it('shows canned DNS rows only after the sample check', async () => {
    render(<DemoSampleDnsCheck />)
    expect(screen.queryByRole('table')).toBeNull()
    await userEvent.click(
      screen.getByRole('button', { name: 'sample.dns.check' }),
    )
    expect(screen.getByRole('table')).toBeTruthy()
    expect(screen.getByText('sample.dns.verified')).toBeTruthy()
  })

  it('renders labeled sample logs', () => {
    render(<DemoSampleLogs />)
    expect(screen.getByLabelText('sample.logs.label')).toBeTruthy()
  })
})
