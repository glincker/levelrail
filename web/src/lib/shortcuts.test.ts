import { afterEach, describe, expect, it } from 'vitest'
import {
  CHORD_TIMEOUT_MS,
  INITIAL_CHORD,
  isDialogOpen,
  isShortcutInputSuppressed,
  stepChord,
  type ChordState,
  type KeyInput,
  type ShortcutAction,
} from './shortcuts'

const base: KeyInput = {
  key: '',
  ctrl: false,
  meta: false,
  alt: false,
  typing: false,
  dialogOpen: false,
}

interface Step {
  input: Partial<KeyInput>
  at: number
  action: ShortcutAction
}

const cases: { name: string; steps: Step[] }[] = [
  {
    name: 'g then a goes to apps',
    steps: [
      { input: { key: 'g' }, at: 0, action: null },
      { input: { key: 'a' }, at: 100, action: { type: 'go', to: '/apps' } },
    ],
  },
  {
    name: 'g then t goes to settings',
    steps: [
      { input: { key: 'g' }, at: 0, action: null },
      { input: { key: 't' }, at: 10, action: { type: 'go', to: '/settings' } },
    ],
  },
  {
    name: 'chord expires after the timeout',
    steps: [
      { input: { key: 'g' }, at: 0, action: null },
      { input: { key: 'a' }, at: CHORD_TIMEOUT_MS + 1, action: null },
    ],
  },
  {
    name: 'chord still valid at the timeout boundary',
    steps: [
      { input: { key: 'g' }, at: 0, action: null },
      {
        input: { key: 'n' },
        at: CHORD_TIMEOUT_MS,
        action: { type: 'go', to: '/nodes' },
      },
    ],
  },
  {
    name: 'second key alone does nothing',
    steps: [{ input: { key: 'a' }, at: 0, action: null }],
  },
  {
    name: 'unknown second key cancels the chord',
    steps: [
      { input: { key: 'g' }, at: 0, action: null },
      { input: { key: 'z' }, at: 10, action: null },
      { input: { key: 'a' }, at: 20, action: null },
    ],
  },
  {
    name: 'question mark does nothing (replaced by the long-press l overlay)',
    steps: [{ input: { key: '?' }, at: 0, action: null }],
  },
  {
    name: 'slash focuses search',
    steps: [{ input: { key: '/' }, at: 0, action: { type: 'focus-search' } }],
  },
  {
    name: 'typing in a field disables shortcuts',
    steps: [{ input: { key: '?', typing: true }, at: 0, action: null }],
  },
  {
    name: 'typing between g and a resets the chord',
    steps: [
      { input: { key: 'g' }, at: 0, action: null },
      { input: { key: 'a', typing: true }, at: 10, action: null },
      { input: { key: 'a' }, at: 20, action: null },
    ],
  },
  {
    name: 'modifier keys disable shortcuts',
    steps: [
      { input: { key: '/', ctrl: true }, at: 0, action: null },
      { input: { key: '/', meta: true }, at: 1, action: null },
      { input: { key: '/', alt: true }, at: 2, action: null },
    ],
  },
  {
    name: 'open dialog disables shortcuts',
    steps: [{ input: { key: '/', dialogOpen: true }, at: 0, action: null }],
  },
]

describe('stepChord', () => {
  it.each(cases)('$name', ({ steps }) => {
    let state: ChordState = INITIAL_CHORD
    for (const step of steps) {
      const res = stepChord(state, { ...base, ...step.input }, step.at)
      expect(res.action).toEqual(step.action)
      state = res.state
    }
  })
})

describe('isDialogOpen', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('is false when no dialog is mounted', () => {
    expect(isDialogOpen()).toBe(false)
  })

  it('is true for a dialog that is actually open', () => {
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('data-open', '')
    document.body.appendChild(dialog)
    expect(isDialogOpen()).toBe(true)
  })

  // Base UI dialogs stay mounted with data-closed (not removed) during and
  // after their exit animation. A dialog in that state must not permanently
  // disable every shortcut, which is exactly what happened before this fix.
  it('is false for a closed dialog that stays mounted for its exit animation', () => {
    const dialog = document.createElement('div')
    dialog.setAttribute('role', 'dialog')
    dialog.setAttribute('data-closed', '')
    document.body.appendChild(dialog)
    expect(isDialogOpen()).toBe(false)
  })
})

describe('stepChord experimental gate', () => {
  const chord = (key: string, enabled: readonly string[]) => {
    const first = stepChord(INITIAL_CHORD, { ...base, key: 'g' }, 0)
    return stepChord(first.state, { ...base, key }, 1, enabled).action
  }
  it.each([
    ['m', [], null],
    ['l', [], null],
    ['m', ['ai-models'], { type: 'go', to: '/models' }],
    ['l', ['load-balancer'], { type: 'go', to: '/loadbalancers' }],
    ['a', [], { type: 'go', to: '/apps' }],
  ])('g %s with %j', (key, enabled, want) => {
    expect(chord(key, enabled)).toEqual(want)
  })
})

describe('stepChord traffic targets', () => {
  const chord = (key: string, has: (to: string) => boolean) => {
    const first = stepChord(INITIAL_CHORD, { ...base, key: 'g' }, 0)
    return stepChord(first.state, { ...base, key }, 1, [], has).action
  }
  it('g r goes to DNS once the route exists', () => {
    expect(chord('r', () => true)).toEqual({ type: 'go', to: '/dns' })
  })
  it('g r does nothing while the DNS route is missing', () => {
    expect(chord('r', () => false)).toBeNull()
  })
  it('g x goes to the proxy page without a route gate', () => {
    expect(chord('x', () => false)).toEqual({
      type: 'go',
      to: '/network/proxy',
    })
  })
})

describe('isShortcutInputSuppressed', () => {
  it('is false for a plain key with no modifier, not typing, no dialog', () => {
    expect(isShortcutInputSuppressed(base)).toBe(false)
  })

  it.each([
    ['typing', { typing: true }],
    ['dialogOpen', { dialogOpen: true }],
    ['ctrl', { ctrl: true }],
    ['meta', { meta: true }],
    ['alt', { alt: true }],
  ])('is true when %s', (_name, override) => {
    expect(isShortcutInputSuppressed({ ...base, ...override })).toBe(true)
  })
})
