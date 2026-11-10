import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Alert } from './alert'
import { Breadcrumb } from './breadcrumb'
import { NumberStepper } from './number-stepper'
import { Pagination } from './pagination'
import { ProgressBar, ProgressRing } from './progress-ring'
import { RemovableChip } from './removable-chip'
import { SelectableCard } from './selectable-card'
import { SkeletonSwap } from './skeleton'

vi.mock('../../hooks/useBrand', () => ({ useBrand: vi.fn() }))

// i18n is not initialised in unit tests, so t() returns the key.
describe('ui primitives', () => {
  it('SelectableCard exposes aria-pressed and a check mark when selected', () => {
    const onSelect = vi.fn()
    const { container } = render(
      <SelectableCard selected onSelect={onSelect}>
        Option
      </SelectableCard>,
    )
    const btn = screen.getByRole('button', { name: 'Option' })
    expect(btn).toHaveAttribute('aria-pressed', 'true')
    expect(container.querySelector('svg')).not.toBeNull()
    fireEvent.click(btn)
    expect(onSelect).toHaveBeenCalled()
  })

  it('Alert renders a tonal banner', () => {
    render(<Alert variant="warning">Careful</Alert>)
    expect(screen.getByRole('alert').className).toContain('tone-warning')
  })

  it('SkeletonSwap marks busy while loading', () => {
    const { container } = render(
      <SkeletonSwap loading skeleton={<span>wait</span>}>
        <span>done</span>
      </SkeletonSwap>,
    )
    expect(container.firstElementChild).toHaveAttribute('aria-busy', 'true')
  })

  it('Breadcrumb marks the last item as current', () => {
    render(
      <Breadcrumb
        items={[
          { key: 'a', label: 'Apps' },
          { key: 'b', label: 'Web' },
        ]}
      />,
    )
    expect(screen.getByText('Web')).toHaveAttribute('aria-current', 'page')
  })

  it('Pagination disables previous on the first page', () => {
    const onPageChange = vi.fn()
    render(<Pagination page={1} pageCount={3} onPageChange={onPageChange} />)
    expect(
      screen.getByRole('button', { name: 'pagination.previous' }),
    ).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'pagination.next' }))
    expect(onPageChange).toHaveBeenCalledWith(2)
  })

  it('NumberStepper clamps at the bounds', () => {
    const onChange = vi.fn()
    render(
      <NumberStepper
        value={3}
        min={1}
        max={3}
        onChange={onChange}
        label="Replicas"
      />,
    )
    expect(
      screen.getByRole('button', { name: 'stepper.increase' }),
    ).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'stepper.decrease' }))
    expect(onChange).toHaveBeenCalledWith(2)
  })

  it('RemovableChip calls onRemove', () => {
    const onRemove = vi.fn()
    render(<RemovableChip label="prod" onRemove={onRemove} />)
    fireEvent.click(screen.getByRole('button'))
    expect(onRemove).toHaveBeenCalled()
  })

  it('ProgressRing and ProgressBar report their value', () => {
    render(
      <>
        <ProgressRing value={140} label="Ring" />
        <ProgressBar value={40} label="Bar" />
      </>,
    )
    expect(screen.getByRole('progressbar', { name: 'Ring' })).toHaveAttribute(
      'aria-valuenow',
      '100',
    )
    expect(screen.getByRole('progressbar', { name: 'Bar' })).toBeInTheDocument()
  })
})
