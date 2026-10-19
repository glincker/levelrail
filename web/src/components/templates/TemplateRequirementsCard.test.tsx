import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { TemplateRequirementsCard } from './TemplateRequirementsCard'
import { needsAcknowledgement } from '../../lib/templateRequirements'
import type { TemplateRequirements } from '../../queries/serviceTemplates'

const elevated: TemplateRequirements = {
  level: 'elevated',
  findings: [
    {
      code: 'docker_socket',
      severity: 'elevated',
      services: ['agent'],
      message: 'Mounts the Docker socket',
    },
  ],
  protections: [{ code: 'rollback', label: 'One-step rollback' }],
}

describe('TemplateRequirementsCard', () => {
  it('requires acknowledgement only for elevated templates', () => {
    expect(needsAcknowledgement(elevated)).toBe(true)
    expect(needsAcknowledgement({ ...elevated, level: 'review' })).toBe(false)
    expect(needsAcknowledgement(undefined)).toBe(false)
  })

  it('reports the acknowledgement checkbox for elevated templates', () => {
    const onChange = vi.fn()
    render(
      <TemplateRequirementsCard
        requirements={elevated}
        acknowledged={false}
        onAcknowledgedChange={onChange}
      />,
    )
    fireEvent.click(screen.getByRole('checkbox'))
    expect(onChange).toHaveBeenCalledWith(true)
  })

  it('shows no checkbox for a standard template', () => {
    render(
      <TemplateRequirementsCard
        requirements={{ level: 'standard', findings: [], protections: [] }}
        onAcknowledgedChange={vi.fn()}
      />,
    )
    expect(screen.queryByRole('checkbox')).toBeNull()
  })
})
