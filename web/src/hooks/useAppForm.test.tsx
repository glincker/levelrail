import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { useAppForm } from './useAppForm'

const schema = z.object({ name: z.string().min(3, 'too short') })
type Values = z.infer<typeof schema>

function Probe({ mode }: Readonly<{ mode?: 'onChange' }>) {
  const { register, formState } = useAppForm<Values>(
    schema,
    mode
      ? { mode, defaultValues: { name: '' } }
      : { defaultValues: { name: '' } },
  )
  return (
    <div>
      <input aria-label="name" {...register('name')} />
      <p role="alert">{formState.errors.name?.message}</p>
    </div>
  )
}

describe('useAppForm', () => {
  it('shows no error on the first keystroke before the field is touched', () => {
    render(<Probe />)
    fireEvent.change(screen.getByLabelText('name'), { target: { value: 'a' } })
    expect(screen.getByRole('alert')).toBeEmptyDOMElement()
  })

  it('shows the error after blur, then revalidates on change', async () => {
    render(<Probe />)
    const input = screen.getByLabelText('name')
    fireEvent.change(input, { target: { value: 'a' } })
    fireEvent.blur(input)
    expect(await screen.findByText('too short')).toBeInTheDocument()
    fireEvent.change(input, { target: { value: 'abcd' } })
    await screen.findByRole('alert', { name: '' })
    expect(screen.queryByText('too short')).not.toBeInTheDocument()
  })

  it('lets callers override the mode', async () => {
    render(<Probe mode="onChange" />)
    fireEvent.change(screen.getByLabelText('name'), { target: { value: 'a' } })
    expect(await screen.findByText('too short')).toBeInTheDocument()
  })
})
