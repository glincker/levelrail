import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DashboardSiteDrop } from './DashboardSiteDrop'

vi.mock('../../queries/pagesSites', () => ({
  usePagesSites: () => ({ data: [] }),
}))
vi.mock('../../lib/zipSite', () => ({
  selectionFromDataTransfer: () =>
    Promise.resolve({ kind: 'zip', label: 'site.zip' }),
}))
vi.mock('../pages/NewSiteDialog', () => ({
  NewSiteDialog: ({
    open,
    initialSelection,
  }: {
    open: boolean
    initialSelection: { label: string } | null
  }) => (open ? <p data-testid="dialog">{initialSelection?.label}</p> : null),
}))

describe('DashboardSiteDrop', () => {
  it('opens the publish dialog with a file dropped on the page', async () => {
    render(<DashboardSiteDrop />)
    const dataTransfer = { types: ['Files'], files: [], items: [] }
    fireEvent.dragEnter(window, { dataTransfer })
    expect(screen.getByTestId('dashboard-site-drop')).toBeTruthy()
    fireEvent.drop(window, { dataTransfer })
    await waitFor(() => {
      expect(screen.getByTestId('dialog').textContent).toBe('site.zip')
    })
  })
})
