import { describe, expect, it } from 'vitest'
import type { AppListEntry } from '../types/appDetail'
import { previewPending } from './appPages'

function page(state?: 'queued' | 'rendering' | 'ready' | 'failed') {
  const item = { name: 'a', preview: state ? { state } : undefined }
  return { items: [item as AppListEntry], total: 1, offset: 0 }
}

describe('previewPending', () => {
  it('polls while a render is queued or running', () => {
    expect(previewPending([page('queued')])).toBe(true)
    expect(previewPending([page('rendering')])).toBe(true)
  })
  it('stops when every card is settled', () => {
    expect(previewPending([page('ready'), page('failed'), page()])).toBe(false)
    expect(previewPending(undefined)).toBe(false)
  })
})
