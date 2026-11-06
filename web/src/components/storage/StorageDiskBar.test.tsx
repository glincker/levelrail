import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import storageEn from '../../locales/en/storage.json'
import {
  buildDiskSegments,
  columnWeights,
  hatchUnits,
  quantizeUnits,
  ROW_UNITS,
} from './diskBar'
import { StorageDiskBar, type StorageDiskBarProps } from './StorageDiskBar'
import { StorageVerdict } from './StorageVerdict'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'storage'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, storage: storageEn } },
  interpolation: { escapeValue: false },
})

const GIB = 1024 ** 3

const base: Omit<StorageDiskBarProps, 'onSelect'> = {
  totalBytes: 200 * GIB,
  freeBytes: 100 * GIB,
  categories: {
    appData: { bytes: 40 * GIB },
    images: {
      bytes: 30 * GIB,
      reclaimableBytes: 10 * GIB,
      topItems: [
        { name: 'web:latest', bytes: 4 * GIB },
        { name: 'api:v2', bytes: 3 * GIB },
        { name: 'db:15', bytes: 2 * GIB },
        { name: 'extra:1', bytes: GIB },
      ],
    },
    buildCache: { bytes: 5 * GIB, reclaimableBytes: 5 * GIB },
    containersLogs: { bytes: 0.001 * GIB },
  },
}

function renderBar(props: Partial<StorageDiskBarProps> = {}) {
  const onSelect = vi.fn()
  render(
    <I18nextProvider i18n={i18n}>
      <StorageDiskBar {...base} {...props} onSelect={onSelect} />
    </I18nextProvider>,
  )
  return onSelect
}

describe('disk segments', () => {
  it('derives other from the remainder and keeps a tiny segment', () => {
    const s = buildDiskSegments({
      totalBytes: 200 * GIB,
      freeBytes: 100 * GIB,
      categories: base.categories,
    })
    const other = s.find((x) => x.id === 'other')
    expect(other?.bytes).toBeCloseTo(25 * GIB - 0.001 * GIB, -6)
    expect(s.find((x) => x.id === 'images')?.topItems).toHaveLength(3)
    const units = quantizeUnits(
      columnWeights(s, 200 * GIB, 100 * GIB).map((c) => c.weight),
    )
    expect(units).toHaveLength(6)
    expect(Math.min(...units)).toBeGreaterThanOrEqual(1)
  })

  it('never derives other when a category is unmeasured', () => {
    const s = buildDiskSegments({
      totalBytes: 100,
      freeBytes: 50,
      categories: { appData: { bytes: null }, images: { bytes: 10 } },
    })
    expect(s.map((x) => x.id)).toEqual(['appData', 'images'])
    expect(s[0]?.bytes).toBeNull()
    expect(columnWeights(s, 100, 50)[0]?.weight).toBeGreaterThan(0)
  })

  it('splits the hatch by reclaimable bytes', () => {
    expect(hatchUnits(30, 10)).toBe(67)
    expect(hatchUnits(30, 99)).toBe(ROW_UNITS)
    expect(hatchUnits(30, 0)).toBe(0)
  })
})

describe('StorageDiskBar', () => {
  it('sizes segments with literal classes, no style attribute, summing to 100', () => {
    renderBar()
    const row = screen.getByRole('button', { name: 'App data, 40 GiB' })
      .parentElement as HTMLElement
    expect(row.querySelectorAll('[style]')).toHaveLength(0)
    expect(row.hasAttribute('style')).toBe(false)
    const sum = Array.from(row.children).reduce(
      (n, c) => n + Number(/basis-\[([\d.]+)%\]/.exec(c.className)?.[1]),
      0,
    )
    expect(sum).toBeCloseTo(100, 5)
    expect(screen.getByTestId('hatch-images')).toBeInTheDocument()
    expect(screen.queryByTestId('hatch-appData')).toBeNull()
  })

  it('shows unmeasured categories as such without a size', () => {
    renderBar({
      categories: { appData: { bytes: null }, images: { bytes: 5 } },
    })
    expect(
      screen.getByRole('button', { name: 'App data, Not measured' }),
    ).toBeInTheDocument()
  })

  it('shows the tooltip card on keyboard focus with top 3 items', async () => {
    const user = userEvent.setup()
    renderBar()
    await user.tab()
    await user.tab()
    expect(screen.getByRole('button', { name: /^Images, / })).toHaveFocus()
    expect(await screen.findByText('web:latest')).toBeInTheDocument()
    expect(screen.getByText('db:15')).toBeInTheDocument()
    expect(screen.queryByText('extra:1')).toBeNull()
    expect(screen.getByText(/can be removed safely/)).toBeInTheDocument()
  })

  it('tabs through segments then legend and calls onSelect', async () => {
    const user = userEvent.setup()
    const onSelect = renderBar({ active: 'images' })
    const images = screen.getByRole('button', { name: /^Images, / })
    expect(images).toHaveAttribute('aria-pressed', 'true')
    images.focus()
    await user.keyboard('{Enter}')
    expect(onSelect).toHaveBeenCalledWith('images')
    await user.click(
      screen.getByRole('button', { name: 'Filter: Build cache' }),
    )
    expect(onSelect).toHaveBeenLastCalledWith('buildCache')
    const order = screen.getAllByRole('button').map((b) => b.tagName)
    expect(order.length).toBe(10)
  })
})

describe('StorageVerdict', () => {
  it('renders one sentence with forecast only for a number', () => {
    const props = {
      freeBytes: 42 * GIB,
      totalBytes: 200 * GIB,
      health: 'healthy' as const,
      safeReclaimableBytes: 9 * GIB,
    }
    const { rerender } = render(
      <I18nextProvider i18n={i18n}>
        <StorageVerdict {...props} action={<button>Review</button>} />
      </I18nextProvider>,
    )
    const sentence = () => document.querySelector('p')?.textContent ?? ''
    expect(sentence()).toBe(
      '42 GiB free of 200 GiB. Healthy. 9.0 GiB can be removed safely.',
    )
    rerender(
      <I18nextProvider i18n={i18n}>
        <StorageVerdict {...props} forecastDays={38} />
      </I18nextProvider>,
    )
    expect(sentence()).toMatch(
      /Healthy\. 9\.0 GiB can be removed safely\. Full in about 38 days at the current rate\.$/,
    )
  })
})
