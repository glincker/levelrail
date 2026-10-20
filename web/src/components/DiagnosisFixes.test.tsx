import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { Diagnosis } from '../types/diagnosis'
import { DiagnosisFixes } from './DiagnosisFixes'

const mutate = vi.fn()
const diagnosis: Diagnosis = {
  explanation: 'x',
  suggestion: 'y',
  confidence: 'high',
  matched_signals: [],
  causes: [
    {
      code: 'WRONG_PORT',
      title: 'App listens on a different port than configured',
      explanation: 'Configured for 3000 but listens on 8080.',
      confidence: 'high',
      evidence: [{ source: 'logs', excerpt: 'listening on 8080' }],
      fixes: [
        {
          n: 1,
          label: 'Change the app port from 3000 to 8080',
          kind: 'patch',
          redeploy: true,
          changes: [{ field: 'port', from: '3000', to: '8080' }],
        },
        {
          n: 2,
          label: 'Set API_KEY',
          kind: 'input',
          changes: [
            { field: 'env.API_KEY', from: '', to: '', needs_input: true },
          ],
        },
        {
          n: 3,
          label: 'Fix volume owner',
          kind: 'manual',
          changes: [],
          hint: 'chown the host directory',
        },
      ],
    },
  ],
}

vi.mock('../queries/diagnosis', () => ({
  useDiagnosis: () => ({ data: diagnosis }),
  useApplyDiagnosisFix: () => ({ isPending: false, mutate }),
}))

function previewButton(index: number): HTMLElement {
  const button = screen.getAllByRole('button', { name: /Preview fix/ })[index]
  if (!button) {
    throw new Error(`no preview button at ${index}`)
  }
  return button
}

describe('DiagnosisFixes', () => {
  it('previews a patch fix and applies it', () => {
    render(<DiagnosisFixes appName="web" deployId="d1" />)
    expect(screen.getByText('[logs] listening on 8080')).toBeTruthy()
    expect(screen.getByText('chown the host directory')).toBeTruthy()

    fireEvent.click(previewButton(0))
    expect(screen.getByText('3000 -> 8080')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Apply fix' }))
    expect(mutate).toHaveBeenCalledTimes(1)
    expect(mutate.mock.calls.at(0)?.at(0)).toMatchObject({ redeploy: true })
  })

  it('blocks an input fix until a value is entered', () => {
    render(<DiagnosisFixes appName="web" />)
    fireEvent.click(previewButton(1))
    const apply = screen.getByRole('button', { name: 'Apply fix' })
    expect((apply as HTMLButtonElement).disabled).toBe(true)
    fireEvent.change(screen.getByLabelText('Value for env.API_KEY'), {
      target: { value: 'k' },
    })
    expect((apply as HTMLButtonElement).disabled).toBe(false)
  })
})
