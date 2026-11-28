import { describe, expect, it } from 'vitest'
import type { TFunction } from 'i18next'
import { buildWaitingItems } from './attentionWaiting'

const t = ((key: string) => key) as unknown as TFunction<'attention'>

describe('attention feed announcements', () => {
  it('maps an announcement item to its title and plain-text detail', () => {
    const items = buildWaitingItems({
      now: 0,
      t,
      feed: [
        {
          id: 'announcement:a1',
          severity: 'warning',
          kind: 'announcement',
          subject: 'a1',
          title: 'Patch available',
          detail: 'Update **now** to fix it.',
          action: 'open',
          link: '/settings/announcements',
        },
      ],
    })
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({
      id: 'announcement:a1',
      kind: 'announcement',
      title: 'Patch available',
      detail: 'Update now to fix it.',
    })
  })
})
