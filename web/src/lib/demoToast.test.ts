import { describe, expect, it, vi } from 'vitest'
import {
  createDemoToastGate,
  DEMO_TOAST_ID,
  installUserActionTracking,
  showDemoToast,
} from './demoToast'

function gateAt(clock: { t: number }) {
  return createDemoToastGate({
    intervalMs: 10000,
    actionWindowMs: 2000,
    now: () => clock.t,
  })
}

describe('demo toast gate', () => {
  it('ignores background writes with no recent user action', () => {
    const clock = { t: 100000 }
    expect(gateAt(clock).accept('POST')).toBe(false)
  })

  it('ignores reads even right after a click', () => {
    const clock = { t: 100000 }
    const gate = gateAt(clock)
    gate.noteUserAction()
    expect(gate.accept('GET')).toBe(false)
  })

  it('accepts one user write and rate limits repeats to one per interval', () => {
    const clock = { t: 100000 }
    const gate = gateAt(clock)
    gate.noteUserAction()
    expect(gate.accept('POST')).toBe(true)
    clock.t += 1500
    gate.noteUserAction()
    expect(gate.accept('DELETE')).toBe(false)
    clock.t += 9000
    gate.noteUserAction()
    expect(gate.accept('PUT')).toBe(true)
  })

  it('drops a write that lands long after the click', () => {
    const clock = { t: 100000 }
    const gate = gateAt(clock)
    gate.noteUserAction()
    clock.t += 5000
    expect(gate.accept('POST')).toBe(false)
  })
})

describe('showDemoToast', () => {
  it('closes the previous copy and reuses one stable id', () => {
    const manager = { add: vi.fn(), close: vi.fn() }
    showDemoToast(manager, { title: 'x' })
    showDemoToast(manager, { title: 'x' })
    expect(manager.close).toHaveBeenCalledTimes(2)
    expect(manager.close).toHaveBeenCalledWith(DEMO_TOAST_ID)
    expect(manager.add).toHaveBeenNthCalledWith(2, {
      title: 'x',
      id: DEMO_TOAST_ID,
    })
  })
})

describe('user action tracking', () => {
  it('counts a button click but not a link click', () => {
    const note = vi.fn()
    const stop = installUserActionTracking({
      noteUserAction: note,
      accept: () => true,
    })
    const btn = document.createElement('button')
    const link = document.createElement('a')
    document.body.append(btn, link)
    link.click()
    expect(note).not.toHaveBeenCalled()
    btn.click()
    expect(note).toHaveBeenCalledTimes(1)
    stop()
    btn.remove()
    link.remove()
  })
})
