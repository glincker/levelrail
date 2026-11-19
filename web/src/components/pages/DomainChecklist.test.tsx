import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import DomainChecklist from './DomainChecklist'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}))

describe('DomainChecklist', () => {
  it('lists each check with its hint for assistive tech', () => {
    render(
      <DomainChecklist
        rows={[
          { code: 'net.port80', ok: false, hint: 'port 80 is closed' },
          { code: 'tls.certificate', ok: true, hint: 'issued' },
        ]}
      />,
    )
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText(/port 80 is closed/)).toBeTruthy()
  })

  it('renders nothing without rows', () => {
    const { container } = render(<DomainChecklist rows={[]} />)
    expect(container.firstChild).toBeNull()
  })
})
