import { render, screen } from '@testing-library/react'
import { CheckCircleIcon } from '@phosphor-icons/react/dist/ssr'
import { describe, expect, it } from 'vitest'
import { StatusBadge } from './status-badge'
import { Badge } from './badge'
import { StatusPill } from '@/components/kit'
import { TONE_BADGE_VARIANT, type Tone } from '@/components/kit/tone'

describe('status vocabulary', () => {
  it('resolves every tone to a Badge variant that uses the tone tokens', () => {
    const tones: Tone[] = [
      'neutral',
      'success',
      'warning',
      'danger',
      'info',
      'accent',
    ]
    for (const tone of tones) {
      const { unmount } = render(
        <Badge variant={TONE_BADGE_VARIANT[tone]}>x-{tone}</Badge>,
      )
      const cls = screen.getByText(`x-${tone}`).className
      expect(cls).not.toMatch(/\b(green|amber|red|sky|blue|emerald)-\d/)
      unmount()
    }
  })

  it('StatusBadge and StatusPill share the same tone classes', () => {
    render(
      <>
        <StatusBadge tone="success" label="Live" icon={CheckCircleIcon} />
        <StatusPill tone="success" label="Running" />
      </>,
    )
    expect(screen.getByText('Live').className).toContain('text-tone-success')
    expect(screen.getByText('Running').className).toContain('text-tone-success')
  })

  it('has an info variant on the tone tokens', () => {
    render(<Badge variant="info">Propagating</Badge>)
    expect(screen.getByText('Propagating').className).toContain(
      'bg-tone-info-soft',
    )
  })
})
