import { describe, expect, it } from 'vitest'
import {
  errorsPanelState,
  logsTabState,
  showDomainExtras,
  showPreviewExtras,
  showVolumesEditor,
} from './appTabState'

describe('appTabState', () => {
  it('shows the volumes editor only with volumes or while adding', () => {
    expect(showVolumesEditor(0, false)).toBe(false)
    expect(showVolumesEditor(0, true)).toBe(true)
    expect(showVolumesEditor(2, false)).toBe(true)
  })
  it('shows domain extras only with a domain', () => {
    expect(showDomainExtras(undefined)).toBe(false)
    expect(showDomainExtras([])).toBe(false)
    expect(showDomainExtras(['a.example.com'])).toBe(true)
  })
  it('derives the errors panel state', () => {
    const base = {
      trackingLoading: false,
      trackingOn: true,
      listLoading: false,
      issueCount: 0,
      filtersChanged: false,
    }
    expect(errorsPanelState({ ...base, trackingLoading: true })).toBe('loading')
    expect(errorsPanelState({ ...base, trackingOn: false })).toBe('off')
    expect(errorsPanelState({ ...base, listLoading: true })).toBe('loading')
    expect(errorsPanelState(base)).toBe('empty')
    expect(errorsPanelState({ ...base, issueCount: 3 })).toBe('issues')
    expect(errorsPanelState({ ...base, filtersChanged: true })).toBe('issues')
  })
  it('shows preview extras once previews are on or exist', () => {
    const base = { loading: false, prPreviewsOn: false, previewCount: 0 }
    expect(
      showPreviewExtras({ ...base, loading: true, prPreviewsOn: true }),
    ).toBe(false)
    expect(showPreviewExtras(base)).toBe(false)
    expect(showPreviewExtras({ ...base, prPreviewsOn: true })).toBe(true)
    expect(showPreviewExtras({ ...base, previewCount: 1 })).toBe(true)
  })
  it('shows logs tabs only once the app has deployed', () => {
    const base = { loading: false, attemptCount: 0, conditionCount: 0 }
    expect(logsTabState({ ...base, loading: true, attemptCount: 2 })).toBe(
      'loading',
    )
    expect(logsTabState(base)).toBe('empty')
    expect(logsTabState({ ...base, attemptCount: 1 })).toBe('ready')
    expect(logsTabState({ ...base, conditionCount: 1 })).toBe('ready')
  })
})
