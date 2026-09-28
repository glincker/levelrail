import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { IacPlanView, IacResultView } from './IacPlanView'
import type { IacPlan } from '../queries/iac'

const plan: IacPlan = {
  hash: 'abc',
  summary: { create: 0, update: 1, delete: 0, noop: 1, error: 0 },
  changes: [
    {
      kind: 'App',
      name: 'web',
      action: 'update',
      file: 'infra/web.yaml',
      line: 3,
      fields: [
        { path: 'image', op: 'change', old: 'a:1', new: 'a:2' },
        { path: 'env.LOG', op: 'add', new: '(hidden)' },
      ],
    },
    { kind: 'Tag', name: 'x', action: 'noop' },
  ],
}

describe('IacPlanView', () => {
  it('shows the summary and field diffs, and hides unchanged items', () => {
    render(<IacPlanView plan={plan} />)
    expect(screen.getByText(/1 to update/)).toBeTruthy()
    expect(screen.getByText('App/web')).toBeTruthy()
    expect(screen.getByText(/~ image: a:1 -> a:2/)).toBeTruthy()
    expect(screen.getByText(/\+ env\.LOG: \(hidden\)/)).toBeTruthy()
    expect(screen.queryByText('Tag/x')).toBeNull()
  })

  it('says so when nothing changes', () => {
    render(
      <IacPlanView
        plan={{
          ...plan,
          summary: { create: 0, update: 0, delete: 0, noop: 2, error: 0 },
          changes: [{ kind: 'Tag', name: 'x', action: 'noop' }],
        }}
      />,
    )
    expect(screen.getByText(/already matches/)).toBeTruthy()
  })
})

describe('IacResultView', () => {
  it('lists denied and applied items', () => {
    render(
      <IacResultView
        results={[
          { kind: 'App', name: 'web', action: 'update', status: 'applied' },
          {
            kind: 'App',
            name: 'victim',
            action: 'update',
            status: 'denied',
            error: '403 forbidden',
          },
        ]}
      />,
    )
    expect(screen.getByText('denied')).toBeTruthy()
    expect(screen.getByText('App/victim')).toBeTruthy()
    expect(screen.getByText(/403 forbidden/)).toBeTruthy()
  })
})
