import { describe, expect, it } from 'vitest'
import {
  CHORD_TIMEOUT_MS,
  INITIAL_CHORD,
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
      { input: { key: 'x' }, at: 10, action: null },
      { input: { key: 'a' }, at: 20, action: null },
    ],
  },
  {
    name: 'question mark opens help',
    steps: [{ input: { key: '?' }, at: 0, action: { type: 'help' } }],
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
    steps: [{ input: { key: '?', dialogOpen: true }, at: 0, action: null }],
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
