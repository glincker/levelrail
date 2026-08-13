import { createContext } from 'react'
import type { Brand } from '../types/brand'

// Split out of components/BrandProvider.tsx so that file can export only
// the provider component: react-refresh/only-export-components flags a
// file that exports both a component and a non-component value (a
// context object or a hook), the same reason lib/authStore.ts and
// hooks/useAuthUsername.ts are two files instead of one.
export const BrandContext = createContext<Brand | null>(null)
