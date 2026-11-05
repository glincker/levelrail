import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  APPEARANCE_DEFAULTS,
  getAppearance,
  resetAppearance,
  setAppearance,
} from './appearance'
import { getDensity, setDensity } from './density'
import { getPageWidth, setPageWidth } from './pageWidth'
import {
  buildPayload,
  createAppearanceSync,
  exportAppearanceJSON,
  parseImport,
  parseSynced,
  syncLineKey,
  type SyncApi,
} from './appearanceSync'

function setup(debounceMs = 800) {
  const put = vi.fn<SyncApi['put']>(() => Promise.resolve())
  const del = vi.fn<SyncApi['del']>(() => Promise.resolve())
  const sync = createAppearanceSync(
    { put, del },
    { debounceMs, now: () => 1000 },
  )
  const stop = sync.start()
  return { put, del, sync, stop }
}

beforeEach(() => {
  vi.useRealTimers()
  localStorage.clear()
  resetAppearance()
})

describe('precedence', () => {
  it('account value wins over local and mirrors to storage', () => {
    setAppearance({ font: 'serif' })
    const { sync, put, stop } = setup()
    sync.reconcile({
      v: 1,
      font: 'mono',
      width: 'wide',
      density: 'comfortable',
    })
    expect(getAppearance().font).toBe('mono')
    expect(getPageWidth()).toBe('wide')
    expect(getDensity()).toBe('comfortable')
    expect(localStorage.getItem('levelrail-appearance-v1')).toContain('mono')
    expect(put).not.toHaveBeenCalled()
    expect(sync.getState().status).toBe('synced')
    stop()
  })

  it('first device pushes a non-default local value exactly once', async () => {
    setAppearance({ accent: 'rose' })
    const { sync, put, stop } = setup()
    sync.reconcile(null)
    await sync.flush()
    expect(put).toHaveBeenCalledTimes(1)
    expect(put.mock.calls[0]?.[0]).toMatchObject({ v: 1, accent: 'rose' })
    stop()
  })

  it('does not push when local is default and account is empty', () => {
    const { sync, put, stop } = setup()
    sync.reconcile(null)
    expect(put).not.toHaveBeenCalled()
    stop()
  })
})

describe('debounce', () => {
  it('coalesces rapid edits into one PUT with the full object', async () => {
    vi.useFakeTimers()
    const { sync, put, stop } = setup(800)
    sync.reconcile({ v: 1 })
    setAppearance({ font: 'serif' })
    setPageWidth('wide')
    setDensity('comfortable')
    setAppearance({ textSize: 110 })
    await vi.advanceTimersByTimeAsync(799)
    expect(put).not.toHaveBeenCalled()
    await vi.advanceTimersByTimeAsync(2)
    expect(put).toHaveBeenCalledTimes(1)
    expect(put.mock.calls[0]?.[0]).toMatchObject({
      v: 1,
      font: 'serif',
      textSize: 110,
      width: 'wide',
      density: 'comfortable',
    })
    stop()
  })

  it('does not send edits made while applying the account value', async () => {
    vi.useFakeTimers()
    const { sync, put, stop } = setup()
    sync.reconcile({ font: 'mono' })
    await vi.advanceTimersByTimeAsync(2000)
    expect(put).not.toHaveBeenCalled()
    stop()
  })
})

describe('validation', () => {
  it('clamps a bad server value and keeps unknown keys aside', () => {
    const p = parseSynced({
      darkBase: 'nope',
      textSize: 9999,
      accentCustom: 'zzz',
      width: 'huge',
      density: 7,
      future: { a: 1 },
    })
    expect(p?.appearance.darkBase).toBe(APPEARANCE_DEFAULTS.darkBase)
    expect(p?.appearance.textSize).toBe(140)
    expect(p?.appearance.accentCustom).toBe(APPEARANCE_DEFAULTS.accentCustom)
    expect(p?.width).toBe('comfortable')
    expect(p?.density).toBe('compact')
    expect(p?.extras).toEqual({ future: { a: 1 } })
  })

  it('treats non-objects as absent and sends extras back on PUT', async () => {
    expect(parseSynced('x')).toBeNull()
    expect(parseSynced([1])).toBeNull()
    const { sync, put, stop } = setup()
    sync.reconcile({ font: 'mono', future: 'keep' })
    setAppearance({ accent: 'rose' })
    await sync.flush()
    expect(put.mock.calls[0]?.[0]).toMatchObject({
      future: 'keep',
      accent: 'rose',
    })
    stop()
  })
})

describe('export and import', () => {
  it('round trips', () => {
    setAppearance({ font: 'rounded', textSize: 115 })
    setPageWidth('full')
    const text = exportAppearanceJSON()
    resetAppearance()
    const parsed = parseImport(text)
    expect(parsed?.appearance.font).toBe('rounded')
    expect(parsed?.appearance.textSize).toBe(115)
    expect(parsed?.width).toBe('full')
    expect(buildPayload().v).toBe(1)
  })

  it('carries the personalization fields through the payload and import', () => {
    setAppearance({
      radius: 'sharp',
      contrast: 'on',
      codeFont: 'menlo',
      codeSize: 12,
      contentWidth: 1000,
    })
    setPageWidth('custom')
    const payload = buildPayload()
    expect(payload).toMatchObject({
      radius: 'sharp',
      contrast: 'on',
      codeFont: 'menlo',
      codeSize: 12,
      contentWidth: 1000,
      width: 'custom',
    })
    resetAppearance()
    const parsed = parseImport(JSON.stringify(payload))
    expect(parsed?.width).toBe('custom')
    expect(parsed?.appearance.contentWidth).toBe(1000)
    expect(parsed?.extras).toEqual({})
  })

  it('rejects garbage', () => {
    expect(parseImport('not json')).toBeNull()
    expect(parseImport('[1,2]')).toBeNull()
    expect(parseImport('{"hello":"world"}')).toBeNull()
    expect(parseImport(' '.repeat(70_000))).toBeNull()
  })
})

describe('status', () => {
  it('moves through syncing, synced, failed and too large', async () => {
    const { sync, put, stop } = setup()
    sync.reconcile({ v: 1 })
    setAppearance({ font: 'serif' })
    const pending = sync.flush()
    expect(syncLineKey(sync.getState())).toBe('syncing')
    await pending
    expect(syncLineKey(sync.getState())).toBe('synced')
    put.mockRejectedValueOnce(Object.assign(new Error('x'), { status: 500 }))
    setAppearance({ font: 'mono' })
    await sync.flush()
    expect(syncLineKey(sync.getState())).toBe('notSynced')
    put.mockRejectedValueOnce(Object.assign(new Error('x'), { status: 413 }))
    setAppearance({ font: 'readable' })
    await sync.flush()
    expect(syncLineKey(sync.getState())).toBe('tooLarge')
    sync.setEnabled(false)
    expect(syncLineKey(sync.getState())).toBe('off')
    stop()
  })

  it('reset clears the account value', async () => {
    const { sync, del, stop } = setup()
    sync.reconcile({ v: 1 })
    await sync.clearAccount()
    expect(del).toHaveBeenCalledTimes(1)
    stop()
  })
})
