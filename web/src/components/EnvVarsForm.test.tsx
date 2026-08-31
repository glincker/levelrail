import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { EnvVarsForm } from './EnvVarsForm'

function renderForm(overrides: Partial<Parameters<typeof EnvVarsForm>[0]> = {}) {
  render(
    <EnvVarsForm
      title="Environment variables"
      description="desc"
      emptyMessage="No environment variables set."
      pastePlaceholder="KEY=value"
      values={{ FOO: 'bar' }}
      isPending={false}
      onSave={vi.fn()}
      {...overrides}
    />,
  )
}

describe('EnvVarsForm', () => {
  it('renders no badge by default', () => {
    renderForm()
    expect(screen.queryByText('own value')).not.toBeInTheDocument()
  })

  it('renders renderKeyBadge next to a row, keyed off the live field value', () => {
    renderForm({
      renderKeyBadge: (key) => (key ? <span>badge for {key}</span> : null),
    })
    expect(screen.getByText('badge for FOO')).toBeInTheDocument()
  })

  it('renders inheritedRows as read-only entries separate from the editable list', () => {
    renderForm({
      inheritedRows: [
        { key: 'SHARED', value: 'inherited-value', badge: <span>from project</span> },
      ],
    })
    expect(screen.getByText('SHARED')).toBeInTheDocument()
    expect(screen.getByText('inherited-value')).toBeInTheDocument()
    expect(screen.getByText('from project')).toBeInTheDocument()
    // Only FOO is editable; SHARED must not appear as an <input> value.
    expect(screen.getAllByLabelText('Variable name')).toHaveLength(1)
  })

  it('renders nothing extra when inheritedRows is empty', () => {
    renderForm({ inheritedRows: [] })
    expect(
      screen.queryByText(/inherited from a shared tier/i),
    ).not.toBeInTheDocument()
  })
})
