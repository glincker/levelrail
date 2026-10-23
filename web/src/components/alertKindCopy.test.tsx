import { render, screen } from '@testing-library/react'
import { useTranslation } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { AllI18n } from '@/test/allI18n'
import { AlertKindGuidance } from './AlertKindGuidance'
import { alertKindLabel, isAlertKindCopyKind } from './alertKindCopy'

function LabelProbe({ kind }: { kind: string }) {
  const { t } = useTranslation('observability')
  return <span>{alertKindLabel(t, kind)}</span>
}

describe('alert kind copy', () => {
  it.each([
    ['backup_failed', 'Backup failed'],
    ['deploy_failed', 'Deploy failed'],
    ['database_down', 'Database down'],
    ['database_backup_stale', 'Database backup stale'],
    ['database_disk_high', 'Database disk high'],
    ['database_connections_high', 'Database connections high'],
    ['node_disk_space', 'Node disk space'],
    ['email_delivery_failing', 'Email delivery failing'],
  ])('labels %s as %s', (kind, label) => {
    expect(isAlertKindCopyKind(kind)).toBe(true)
    render(
      <AllI18n>
        <LabelProbe kind={kind} />
      </AllI18n>,
    )
    expect(screen.getByText(label)).toBeInTheDocument()
  })

  it('falls back to the raw kind for kinds without copy', () => {
    expect(isAlertKindCopyKind('export_behind')).toBe(false)
    render(
      <AllI18n>
        <LabelProbe kind="export_behind" />
      </AllI18n>,
    )
    expect(screen.getByText('export_behind')).toBeInTheDocument()
  })

  it('renders the description and what-to-do hint', () => {
    render(
      <AllI18n>
        <AlertKindGuidance kind="deploy_failed" />
      </AllI18n>,
    )
    expect(
      screen.getByText(
        'Fires when a deploy stops before the new release serves traffic.',
      ),
    ).toBeInTheDocument()
    expect(screen.getByText('What to do:')).toBeInTheDocument()
  })

  it('omits the description when withDescription is false', () => {
    render(
      <AllI18n>
        <AlertKindGuidance kind="database_down" withDescription={false} />
      </AllI18n>,
    )
    expect(
      screen.queryByText(
        'Fires while the database is not accepting connections.',
      ),
    ).not.toBeInTheDocument()
    expect(screen.getByText('What to do:')).toBeInTheDocument()
  })
})
