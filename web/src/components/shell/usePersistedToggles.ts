import * as React from 'react'
import { isBoolRecord, readJson, writeJson } from './persist'

export function usePersistedToggles(
  key: string,
): [Record<string, boolean>, (id: string, open: boolean) => void] {
  const [state, setState] = React.useState<Record<string, boolean>>(() =>
    readJson(key, {}, isBoolRecord),
  )
  const set = React.useCallback(
    (id: string, open: boolean) => {
      setState((prev) => {
        const next = { ...prev, [id]: open }
        writeJson(key, next)
        return next
      })
    },
    [key],
  )
  return [state, set]
}
