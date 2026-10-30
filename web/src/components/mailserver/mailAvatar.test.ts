import { describe, expect, it } from 'vitest'
import { avatarInitials, avatarTone } from './mailAvatar'
import { nextStepOf } from './mailSetup'
import type { MailStatus } from '@/queries/mailStatus'

const up: MailStatus = {
  installed: true,
  container_state: 'running',
  admin_reachable: true,
  tls: 'provided',
  next_step: '',
}

describe('avatars', () => {
  it('keeps one tone per address, case-insensitively', () => {
    expect(avatarTone('Jo@example.com')).toBe(avatarTone('jo@example.com'))
  })
  it('spreads addresses over more than one tone', () => {
    const tones = new Set(
      ['a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'].map((x) =>
        avatarTone(`${x}@x.io`),
      ),
    )
    expect(tones.size).toBeGreaterThan(1)
  })
  it('builds initials from separators or the first two letters', () => {
    expect(avatarInitials('ann.lee')).toBe('AL')
    expect(avatarInitials('jo')).toBe('JO')
  })
})

describe('nextStepOf without a DNS report', () => {
  const facts = { domains: 1, mailboxes: 2, backups: 1 }
  it('settles once domain, mailbox and backup exist', () => {
    expect(nextStepOf(up, undefined, facts)).toBeNull()
  })
  it('still asks for DNS while something is missing', () => {
    expect(nextStepOf(up, undefined, { ...facts, backups: 0 })).toBe('dns')
    expect(nextStepOf(up, undefined)).toBe('dns')
  })
})
