import * as React from 'react'
import { useSearch, useSearchSuggest } from '../queries/search'
import { parseMode } from '../lib/paletteMode'
import {
  buildServerQuery,
  mergeChips,
  parseInput,
  trailingToken,
  type Chip,
} from '../lib/paletteQuery'

const DEBOUNCE_MS = 100
const SEARCH_LIMIT = 60
const MIN_SUGGEST_VALUE = 1

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = React.useState(value)
  React.useEffect(() => {
    const id = window.setTimeout(() => setV(value), ms)
    return () => window.clearTimeout(id)
  }, [value, ms])
  return v
}

// Input state (chips plus free text) and the debounced server search for the palette.
export function usePaletteSearch(open: boolean) {
  const [chips, setChips] = React.useState<Chip[]>([])
  const [text, setText] = React.useState('')

  const serverQuery = buildServerQuery(chips, text)
  const debounced = useDebounced(serverQuery, DEBOUNCE_MS)
  const inMode = parseMode(text) !== null
  const search = useSearch(
    { q: debounced, limit: SEARCH_LIMIT },
    open && debounced !== '' && !inMode,
  )

  const token = trailingToken(text)
  const suggestQuery =
    token?.key && token.value.length >= MIN_SUGGEST_VALUE
      ? `${token.key}:${token.value}`
      : ''
  const debouncedSuggest = useDebounced(suggestQuery, DEBOUNCE_MS)
  const suggest = useSearchSuggest(
    debouncedSuggest,
    open && debouncedSuggest !== '' && debouncedSuggest === suggestQuery,
  )

  const setInput = React.useCallback((value: string) => {
    const parsed = parseInput(value)
    if (parsed.chips.length > 0) {
      setChips((prev) => mergeChips(prev, parsed.chips))
    }
    setText(parsed.text)
  }, [])

  const reset = React.useCallback(() => {
    setChips([])
    setText('')
  }, [])

  const removeChip = React.useCallback((chip: Chip) => {
    setChips((prev) =>
      prev.filter((c) => !(c.key === chip.key && c.value === chip.value)),
    )
  }, [])

  const popChip = React.useCallback(() => {
    setChips((prev) => prev.slice(0, -1))
  }, [])

  // Replaces the whole input, e.g. when a saved or recent search is picked.
  const applyQuery = React.useCallback((q: string) => {
    const parsed = parseInput(`${q} `)
    setChips(parsed.chips)
    setText(parsed.text)
  }, [])

  const acceptScope = React.useCallback((chip: Chip) => {
    setChips((prev) => mergeChips(prev, [chip]))
    setText((prev) => {
      const t = trailingToken(prev)
      return t ? prev.slice(0, t.start) : prev
    })
  }, [])

  const suggestData = suggest.data
  return React.useMemo(
    () => ({
      chips,
      text,
      serverQuery,
      search,
      suggestions: Array.isArray(suggestData) ? suggestData : [],
      token,
      setInput,
      reset,
      removeChip,
      popChip,
      applyQuery,
      acceptScope,
    }),
    [
      chips,
      text,
      serverQuery,
      search,
      suggestData,
      token,
      setInput,
      reset,
      removeChip,
      popChip,
      applyQuery,
      acceptScope,
    ],
  )
}
