import { afterEach, describe, expect, it } from 'vitest'
import { setDemoInfo } from './demoStore'
import { buildDemoRepoPlan, repoNameFromUrl } from './demoImportSample'
import { planImport } from '../queries/imports'

describe('demo import sample', () => {
  afterEach(() => {
    setDemoInfo({ enabled: false })
  })

  it('derives the app name from any repo URL', () => {
    expect(repoNameFromUrl('https://github.com/acme/Cool_App.git')).toBe(
      'cool-app',
    )
    expect(buildDemoRepoPlan('github.com/acme/shop').suggested_name).toBe(
      'shop',
    )
  })

  it('returns a canned plan in demo mode without calling the API', async () => {
    setDemoInfo({ enabled: true })
    const plan = await planImport({ text: 'https://github.com/acme/shop' })
    expect(plan.suggested_name).toBe('shop')
    expect(plan.sample?.buildLog.length).toBeGreaterThan(0)
  })
})
