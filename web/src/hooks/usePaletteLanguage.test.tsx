import { renderHook } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { createTestQueryClient } from '@/test/testQueryClient'
import { usePaletteLanguage } from './usePaletteLanguage'

const changeLanguage = vi.fn(() => Promise.resolve())
const i18n = {
  language: 'en',
  resolvedLanguage: 'en',
  options: { supportedLngs: ['cimode', 'en', 'es'] },
  changeLanguage,
}
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, opts?: { name?: string }) =>
      key === 'language.switchTo' ? `Language: ${opts?.name ?? ''}` : key,
    i18n,
  }),
}))
vi.mock('../queries/accountSelf', () => ({
  preferencesQueryOptions: () => ({
    queryKey: ['prefs'],
    queryFn: () => Promise.resolve({ language: 'en' }),
  }),
  useSavePreferences: () => ({ mutate: vi.fn() }),
}))

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={createTestQueryClient()}>
    {children}
  </QueryClientProvider>
)

describe('usePaletteLanguage', () => {
  it('builds the step and the searchable entries from supportedLngs', () => {
    const { result } = renderHook(() => usePaletteLanguage(vi.fn()), {
      wrapper,
    })
    expect(result.current.step.map((i) => i.key)).toEqual([
      'language-step-en',
      'language-step-es',
    ])
    const keys = result.current.base.map((i) => i.key)
    expect(keys).toContain('action-change-language')
    expect(keys).toContain('language-es')
    const es = result.current.base.find((i) => i.key === 'language-es')
    expect(es?.keywords).toContain('Spanish')
    expect(es?.keywords).toContain('language.keywords')
  })

  it('opens the second step without closing, and switches language at once', () => {
    const openStep = vi.fn()
    const { result } = renderHook(() => usePaletteLanguage(openStep), {
      wrapper,
    })
    const opener = result.current.base.find(
      (i) => i.key === 'action-change-language',
    )
    expect(opener?.keepOpen).toBe(true)
    opener?.run()
    expect(openStep).toHaveBeenCalledOnce()
    result.current.step.find((i) => i.key === 'language-step-es')?.run()
    expect(changeLanguage).toHaveBeenCalledWith('es')
  })
})
