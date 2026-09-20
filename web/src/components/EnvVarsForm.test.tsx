import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { EnvVarsForm } from './EnvVarsForm'

function renderForm(
  overrides: Partial<Parameters<typeof EnvVarsForm>[0]> = {},
) {
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
        {
          key: 'SHARED',
          value: 'inherited-value',
          badge: <span>from project</span>,
        },
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

  it('imports a browsed .env file into the field list via the paste dialog', async () => {
    const user = userEvent.setup()
    renderForm({ values: {} })

    await user.click(screen.getByRole('button', { name: 'Paste .env' }))
    const fileInput = screen.getByLabelText('Upload .env file')
    const file = new File(
      ['UPLOADED_KEY=uploaded-value\n# comment\n'],
      'app.env',
      {
        type: 'text/plain',
      },
    )

    await user.upload(fileInput, file)

    await waitFor(() => {
      expect(screen.getByDisplayValue('UPLOADED_KEY')).toBeInTheDocument()
    })
    expect(screen.getByDisplayValue('uploaded-value')).toBeInTheDocument()
    // The dialog closes itself once a file import stages rows successfully.
    expect(
      screen.queryByRole('heading', { name: 'Paste .env' }),
    ).not.toBeInTheDocument()
  })

  it('shows an error and keeps the dialog open when the uploaded file has no key=value pairs', async () => {
    const user = userEvent.setup()
    renderForm({ values: {} })

    await user.click(screen.getByRole('button', { name: 'Paste .env' }))
    const fileInput = screen.getByLabelText('Upload .env file')
    const file = new File(['# only a comment\n'], 'empty.env', {
      type: 'text/plain',
    })

    await user.upload(fileInput, file)

    await waitFor(() => {
      expect(
        screen.getByText('No key=value pairs found in empty.env.'),
      ).toBeInTheDocument()
    })
    expect(
      screen.getByRole('heading', { name: 'Paste .env' }),
    ).toBeInTheDocument()
  })
})
