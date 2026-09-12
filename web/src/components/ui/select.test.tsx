import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from './select'

function triggerById(id: string): Element {
  const el = document.body.querySelector(`#${id}`)
  if (!el) throw new Error(`no trigger with id ${id}`)
  return el
}

// Mirrors PromoteAppDialog.test.tsx's own pickOption: base-ui's Select.Item
// only commits a plain click as a real selection once a pointerdown on that
// same item preceded it.
async function pickOption(triggerId: string, optionText: string) {
  const trigger = triggerById(triggerId)
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(trigger)
    try {
      const option = screen.getByText(optionText)
      fireEvent.pointerDown(option, { pointerType: 'mouse' })
      fireEvent.click(option)
      return
    } catch (err) {
      if (attempt === maxAttempts) throw err
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
}

describe('Select', () => {
  it('shows the matched item label on the closed trigger, not the raw value', () => {
    render(
      <Select value="cpu_percent" onValueChange={() => {}}>
        <SelectTrigger id="metric-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="cpu_percent">CPU usage (%)</SelectItem>
          <SelectItem value="memory_usage_bytes">Memory usage</SelectItem>
        </SelectContent>
      </Select>,
    )

    expect(triggerById('metric-select')).toHaveTextContent('CPU usage (%)')
    expect(triggerById('metric-select')).not.toHaveTextContent('cpu_percent')
  })

  it('updates the closed trigger label after picking a different item', async () => {
    function Harness() {
      const [value, setValue] = useState('cpu_percent')
      return (
        <Select value={value} onValueChange={(next) => setValue(next ?? '')}>
          <SelectTrigger id="metric-select">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="cpu_percent">CPU usage (%)</SelectItem>
            <SelectItem value="memory_usage_bytes">Memory usage</SelectItem>
          </SelectContent>
        </Select>
      )
    }
    render(<Harness />)

    expect(triggerById('metric-select')).toHaveTextContent('CPU usage (%)')
    await pickOption('metric-select', 'Memory usage')
    expect(triggerById('metric-select')).toHaveTextContent('Memory usage')
  })

  it('shows the placeholder when nothing is selected yet', () => {
    render(
      <Select value={undefined} onValueChange={() => {}}>
        <SelectTrigger id="metric-select">
          <SelectValue placeholder="Choose a metric" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="cpu_percent">CPU usage (%)</SelectItem>
        </SelectContent>
      </Select>,
    )

    expect(triggerById('metric-select')).toHaveTextContent('Choose a metric')
  })

  it('resolves the label for items nested inside a SelectGroup', () => {
    render(
      <Select value="ghcr" onValueChange={() => {}}>
        <SelectTrigger id="registry-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="docker-hub">Docker Hub</SelectItem>
          {[
            <SelectItem key="ghcr" value="ghcr">
              GitHub Container Registry (GHCR)
            </SelectItem>,
          ]}
        </SelectContent>
      </Select>,
    )

    expect(triggerById('registry-select')).toHaveTextContent(
      'GitHub Container Registry (GHCR)',
    )
  })

  it('lets an explicit items prop override the auto-derived labels', () => {
    render(
      <Select
        value="cpu_percent"
        onValueChange={() => {}}
        items={{ cpu_percent: 'Explicit CPU label' }}
      >
        <SelectTrigger id="metric-select">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="cpu_percent">CPU usage (%)</SelectItem>
        </SelectContent>
      </Select>,
    )

    expect(triggerById('metric-select')).toHaveTextContent('Explicit CPU label')
  })

  it('lets an explicit SelectValue children function override the auto-derived label', () => {
    render(
      <Select value="custom" onValueChange={() => {}}>
        <SelectTrigger id="host-preset-select">
          <SelectValue>
            {(value: string) => (value === 'custom' ? 'Custom (fn)' : value)}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="docker-hub">Docker Hub</SelectItem>
          <SelectItem value="custom">Custom</SelectItem>
        </SelectContent>
      </Select>,
    )

    expect(triggerById('host-preset-select')).toHaveTextContent('Custom (fn)')
  })
})
