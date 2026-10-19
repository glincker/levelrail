import { useCallback } from 'react'
import { useNavigate } from '@tanstack/react-router'

export const APP_PALETTE_ACTIONS = [
  'deploy',
  'restart',
  'stop',
  'rollback',
] as const
export type AppPaletteAction = (typeof APP_PALETTE_ACTIONS)[number]

export const DATABASE_PALETTE_ACTIONS = ['backup'] as const
export type DatabasePaletteAction = (typeof DATABASE_PALETTE_ACTIONS)[number]

function pick<T extends string>(
  allowed: readonly T[],
  value: unknown,
): T | undefined {
  return allowed.find((a) => a === value)
}

export function parseAppAction(value: unknown): AppPaletteAction | undefined {
  return pick(APP_PALETTE_ACTIONS, value)
}

export function parseDatabaseAction(
  value: unknown,
): DatabasePaletteAction | undefined {
  return pick(DATABASE_PALETTE_ACTIONS, value)
}

/** Drops ?action= from the current URL so a reload never re-opens the dialog. */
export function useClearActionParam() {
  const navigate = useNavigate()
  return useCallback(() => {
    void navigate({
      to: '.',
      search: (prev: Record<string, unknown>) => ({
        ...prev,
        action: undefined,
      }),
      replace: true,
    })
  }, [navigate])
}
