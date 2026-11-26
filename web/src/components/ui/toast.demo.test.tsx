import { afterEach, describe, expect, it } from 'vitest'
import { toast } from './toast'
import { noteDemoRefusal } from '@/lib/demoToast'
import { setDemoInfo } from '@/lib/demoStore'

describe('toast in demo mode', () => {
  afterEach(() => {
    setDemoInfo({ enabled: false })
  })

  it('drops an error toast right after a demo refusal but keeps others', () => {
    setDemoInfo({ enabled: true })
    noteDemoRefusal()
    expect(toast.add({ title: 'failed', type: 'error' })).toBe('')
    expect(toast.add({ title: 'ok', type: 'success' })).not.toBe('')
  })

  it('keeps error toasts outside demo mode', () => {
    noteDemoRefusal()
    expect(toast.add({ title: 'failed', type: 'error' })).not.toBe('')
  })
})
