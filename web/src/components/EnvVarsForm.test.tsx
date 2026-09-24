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

    // The file is previewed first, nothing is staged until Import.
    await waitFor(() => {
      expect(
        screen.getByRole('table', { name: 'Import preview' }),
      ).toBeInTheDocument()
    })
    expect(screen.queryByDisplayValue('UPLOADED_KEY')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Import' }))

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

  it('previews new, changed and unchanged rows and honours keep-existing', async () => {
    const user = userEvent.setup()
    renderForm({ values: { FOO: 'bar', SAME: 's' } })

    await user.click(screen.getByRole('button', { name: 'Paste .env' }))
    fireEvent.change(screen.getByLabelText('Paste .env content'), {
      target: { value: 'FOO=changed\nSAME=s\nFRESH=f\n' },
    })

    const table = screen.getByRole('table', { name: 'Import preview' })
    expect(table).toHaveTextContent('FRESH')
    expect(table).toHaveTextContent('Changed')
    expect(table).toHaveTextContent('Unchanged')

    await user.click(screen.getByLabelText('Keep existing values'))
    expect(table).toHaveTextContent('Kept existing')
    await user.click(screen.getByRole('button', { name: 'Import' }))

    expect(screen.getByDisplayValue('FRESH')).toBeInTheDocument()
    expect(screen.getByDisplayValue('bar')).toBeInTheDocument()
    expect(screen.queryByDisplayValue('changed')).not.toBeInTheDocument()
  })

  it('shows a diff of unsaved changes and hides it when nothing changed', async () => {
    const user = userEvent.setup()
    renderForm({ values: { FOO: 'bar', GONE: 'x' } })
    expect(screen.queryByTestId('env-pending-diff')).not.toBeInTheDocument()

    await user.clear(screen.getAllByLabelText('Variable value')[0]!)
    await user.type(screen.getAllByLabelText('Variable value')[0]!, 'baz')
    await user.click(
      screen.getAllByRole('button', { name: 'Remove variable' })[1]!,
    )
    await user.click(screen.getByRole('button', { name: 'Add variable' }))
    await user.type(screen.getAllByLabelText('Variable name')[1]!, 'NEW')

    const diff = await screen.findByTestId('env-pending-diff')
    expect(diff).toHaveTextContent('Added 1')
    expect(diff).toHaveTextContent('Changed 1')
    expect(diff).toHaveTextContent('Removed 1')
  })

  it('exports without secret values', async () => {
    const user = userEvent.setup()
    let exported = ''
    const createObjectURL = vi.fn((blob: Blob) => {
      void blob.text().then((t) => {
        exported = t
      })
      return 'blob:x'
    })
    Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() })
    renderForm({
      values: { FOO: 'bar', API_KEY: 'leaky' },
      exportFilename: 'web.env',
      exportSecretKeys: ['API_KEY'],
    })

    await user.click(screen.getByRole('button', { name: 'Export .env' }))
    await waitFor(() => {
      expect(exported).toContain('FOO=bar')
    })
    expect(exported).toContain('# secret, value not exported\nAPI_KEY=\n')
    expect(exported).not.toContain('leaky')
  })
})
