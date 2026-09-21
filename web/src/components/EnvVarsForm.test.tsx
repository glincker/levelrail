import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { UserEvent } from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { EnvVarsForm } from './EnvVarsForm'

// Same open+pick shape PromoteAppDialog.test.tsx's own pickOption
// documents: fireEvent opens the trigger (unaffected by base-ui's
// pointer-events:none guard during popup positioning), userEvent
// commits the pick (a bare fireEvent.click on the item is a no-op for
// base-ui's Select.Item in jsdom). Retried since base-ui can
// occasionally drop the interaction under concurrent test-file load.
async function pickSharedVarOption(user: UserEvent, optionText: string) {
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(document.getElementById('shared-env-var-picker')!)
    try {
      const items = Array.from(
        document.body.querySelectorAll('[data-slot="select-item"]'),
      )
      const match = items.find((el) => el.textContent?.includes(optionText))
      if (!match) throw new Error(`no select option containing "${optionText}"`)
      await user.click(match)
      return
    } catch (err) {
      if (attempt === maxAttempts) throw err
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
}

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

  it('does not render the shared variable picker when none are available', () => {
    renderForm()
    expect(
      screen.queryByLabelText('Reference a shared variable'),
    ).not.toBeInTheDocument()
  })

  it('renders the shared variable picker when availableSharedVars is set', () => {
    renderForm({
      availableSharedVars: [
        {
          key: 'API_URL',
          tier: 'project',
          secret: false,
          value: 'https://api.example.com',
        },
      ],
    })
    expect(
      screen.getByLabelText('Reference a shared variable'),
    ).toBeInTheDocument()
  })

  it('picking a plain shared variable appends a row prefilled with its current value', async () => {
    const user = userEvent.setup()
    renderForm({
      availableSharedVars: [
        {
          key: 'API_URL',
          tier: 'project',
          secret: false,
          value: 'https://api.example.com',
        },
      ],
    })

    await pickSharedVarOption(user, 'API_URL')

    const keys = screen.getAllByLabelText<HTMLInputElement>('Variable name')
    const values = screen.getAllByLabelText<HTMLInputElement>('Variable value')
    const index = keys.findIndex((el) => el.value === 'API_URL')
    expect(index).toBeGreaterThanOrEqual(0)
    expect(values[index]?.value).toBe('https://api.example.com')
  })

  it('picking a secret shared variable appends a row with an empty value', async () => {
    const user = userEvent.setup()
    renderForm({
      availableSharedVars: [
        { key: 'DB_PASSWORD', tier: 'environment', secret: true },
      ],
    })

    await pickSharedVarOption(user, 'DB_PASSWORD')

    const keys = screen.getAllByLabelText<HTMLInputElement>('Variable name')
    const values = screen.getAllByLabelText<HTMLInputElement>('Variable value')
    const index = keys.findIndex((el) => el.value === 'DB_PASSWORD')
    expect(index).toBeGreaterThanOrEqual(0)
    expect(values[index]?.value).toBe('')
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
