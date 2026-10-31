import { describe, expect, it, vi } from 'vitest'
import {
  buildMailPaletteItems,
  type MailPaletteInput,
} from './mailPaletteItems'

const base = (over: Partial<MailPaletteInput> = {}): MailPaletteInput => ({
  available: true,
  up: true,
  canWrite: true,
  webmail: 'https://mail.example.com/webmail/',
  labels: {
    add: 'Mail: Add mailbox',
    backup: 'Mail: Back up now',
    webmail: 'Mail: Open webmail',
    queue: 'Mail: Open queue',
    keywords: 'mail',
  },
  go: vi.fn(),
  backup: vi.fn(),
  openUrl: vi.fn(),
  ...over,
})

const keys = (i: MailPaletteInput) => buildMailPaletteItems(i).map((x) => x.key)

describe('buildMailPaletteItems', () => {
  it('offers all four actions when mail is installed and up', () => {
    expect(keys(base())).toEqual([
      'action-mail-add',
      'action-mail-backup',
      'action-mail-webmail',
      'action-mail-queue',
    ])
  })

  it('offers nothing when the flag is off or mail is not installed', () => {
    expect(keys(base({ available: false }))).toEqual([])
  })

  it('hides webmail without a url and write actions without permission', () => {
    expect(keys(base({ webmail: undefined }))).not.toContain(
      'action-mail-webmail',
    )
    expect(keys(base({ canWrite: false }))).toEqual([
      'action-mail-webmail',
      'action-mail-queue',
    ])
  })

  it('hides actions while the server is not up', () => {
    expect(keys(base({ up: false }))).toEqual(['action-mail-webmail'])
  })

  it('runs navigation, the backup mutation and the webmail link', () => {
    const i = base()
    const run = (k: string) =>
      buildMailPaletteItems(i)
        .find((x) => x.key === k)
        ?.run()
    run('action-mail-add')
    expect(i.go).toHaveBeenCalledWith({ tab: 'mailboxes', add: true })
    run('action-mail-queue')
    expect(i.go).toHaveBeenCalledWith({ tab: 'overview', queue: true })
    run('action-mail-backup')
    expect(i.backup).toHaveBeenCalled()
    run('action-mail-webmail')
    expect(i.openUrl).toHaveBeenCalledWith('https://mail.example.com/webmail/')
  })
})
