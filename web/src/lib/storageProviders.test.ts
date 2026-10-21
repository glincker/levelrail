import { describe, expect, it } from 'vitest'
import {
  STORAGE_PRESET_LABEL,
  logoIdForStoragePreset,
} from './storageProviders'
import { storagePresetBrand } from './providerBrands'

describe('gcs storage preset', () => {
  it('has a label and the Cloud Storage brand mark', () => {
    expect(STORAGE_PRESET_LABEL.gcs).toBe('Google Cloud Storage')
    expect(logoIdForStoragePreset('gcs')).toBe('google-cloud-storage')
    expect(storagePresetBrand('gcs')).toBe('google-cloud-storage')
  })
})
