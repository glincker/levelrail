import { useContext } from 'react'
import { BrandContext } from '../lib/brandContext'
import type { Brand } from '../types/brand'

// Throws rather than returning a nullable Brand: every consumer renders
// inside routes/__root.tsx's <BrandProvider>, so a null context value
// here means the provider was skipped, a real wiring bug worth failing
// loudly on rather than papering over with a fallback brand.
export function useBrand(): Brand {
  const brand = useContext(BrandContext)
  if (!brand) {
    throw new Error('useBrand must be used within a BrandProvider')
  }
  return brand
}
