import { createContext, useContext } from 'react'

// More options provides this so every nested control renders as a single
// line row; the default (detail sheet, access tab) keeps the card look.
const CompactSettingsContext = createContext(false)
export const CompactSettingsProvider = CompactSettingsContext.Provider
export function useCompactSettings(): boolean {
  return useContext(CompactSettingsContext)
}
