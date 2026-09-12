import { useState } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  DurationInput,
  composeGoDuration,
  parseGoDuration,
} from './duration-input'

describe('parseGoDuration / composeGoDuration round-trip', () => {
  it('parses a bare seconds value', () => {
    expect(parseGoDuration('90s')).toEqual({ amount: '90', unit: 'seconds' })
  })

  it('parses a bare minutes value', () => {
    expect(parseGoDuration('5m')).toEqual({ amount: '5', unit: 'minutes' })
  })

  it('parses a bare hours value', () => {
    expect(parseGoDuration('2h')).toEqual({ amount: '2', unit: 'hours' })
  })

  it('composes an amount and unit back into a Go duration string', () => {
    expect(composeGoDuration('5', 'minutes')).toBe('5m')
    expect(composeGoDuration('90', 'seconds')).toBe('90s')
    expect(composeGoDuration('2', 'hours')).toBe('2h')
  })

  it('round-trips compose(parse(x)) back to x for a single-unit value', () => {
    for (const value of ['90s', '5m', '2h', '0.5s']) {
      const parsed = parseGoDuration(value)
      expect(parsed).not.toBeNull()
      expect(composeGoDuration(parsed!.amount, parsed!.unit)).toBe(value)
    }
  })

  it('collapses a compound duration to its total in the coarsest whole unit', () => {
    expect(parseGoDuration('90m')).toEqual({ amount: '90', unit: 'minutes' })
    // 1h30m = 5400s, which divides evenly into minutes (90) but not
    // hours, so it resolves to minutes rather than falling all the way
    // back to seconds.
    expect(parseGoDuration('1h30m')).toEqual({ amount: '90', unit: 'minutes' })
    expect(parseGoDuration('3600s')).toEqual({ amount: '3600', unit: 'seconds' })
  })

  it('converts a sub-second unit into seconds', () => {
    expect(parseGoDuration('500ms')).toEqual({ amount: '0.5', unit: 'seconds' })
  })

  it('returns null for an empty or unparseable string', () => {
    expect(parseGoDuration('')).toBeNull()
    expect(parseGoDuration('   ')).toBeNull()
    expect(parseGoDuration('garbage')).toBeNull()
  })

  it('composes blank/non-numeric amounts to an empty string', () => {
    expect(composeGoDuration('', 'minutes')).toBe('')
    expect(composeGoDuration('  ', 'seconds')).toBe('')
  })
})

// Mirrors RegistryImagePicker.test.tsx's own pickOption: retries the
// open+click pair on base-ui's Select, since a same-tick click can be
// dropped under concurrent test-file load.
async function pickOption(triggerRole: string, optionText: string) {
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(screen.getByRole(triggerRole))
    try {
      fireEvent.click(screen.getByText(optionText))
      return
    } catch (err) {
      if (attempt === maxAttempts) throw err
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
}

function ControlledDurationInput({ initial }: { initial: string }) {
  const [value, setValue] = useState(initial)
  return (
    <>
      <DurationInput id="test-duration" value={value} onChange={setValue} />
      <output data-testid="raw-value">{value}</output>
    </>
  )
}

describe('DurationInput', () => {
  it('renders the parsed amount and unit for an existing value', () => {
    render(<ControlledDurationInput initial="90s" />)
    expect(screen.getByDisplayValue('90')).toBeInTheDocument()
    expect(screen.getByText('Seconds')).toBeInTheDocument()
  })

  it('renders blank for an empty (optional, unset) value', () => {
    render(<ControlledDurationInput initial="" />)
    expect(screen.getByDisplayValue('')).toBeInTheDocument()
    expect(screen.getByText('Minutes')).toBeInTheDocument()
  })

  it('composes a new Go duration string as the amount changes', async () => {
    render(<ControlledDurationInput initial="5m" />)
    const amountInput = screen.getByDisplayValue('5')
    fireEvent.change(amountInput, { target: { value: '10' } })
    await waitFor(() => {
      expect(screen.getByTestId('raw-value').textContent).toBe('10m')
    })
  })

  it('composes a new Go duration string as the unit changes', async () => {
    render(<ControlledDurationInput initial="5m" />)
    await pickOption('combobox', 'Hours')
    await waitFor(() => {
      expect(screen.getByTestId('raw-value').textContent).toBe('5h')
    })
  })
})
