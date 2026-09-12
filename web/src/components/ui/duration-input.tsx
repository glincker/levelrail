import { useEffect, useRef, useState } from 'react'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

export type DurationUnit = 'seconds' | 'minutes' | 'hours'

const UNIT_SUFFIX: Record<DurationUnit, string> = {
  seconds: 's',
  minutes: 'm',
  hours: 'h',
}

const UNIT_LABEL: Record<DurationUnit, string> = {
  seconds: 'Seconds',
  minutes: 'Minutes',
  hours: 'Hours',
}

// Every unit time.ParseDuration accepts, in seconds, so a value using a
// sub-second unit (or a compound duration like "1h30m") still parses
// into *some* amount+unit pair instead of being dropped.
const SUFFIX_TO_SECONDS: Record<string, number> = {
  ns: 1e-9,
  'µs': 1e-6,
  us: 1e-6,
  ms: 1e-3,
  s: 1,
  m: 60,
  h: 3600,
}

const DURATION_TERM_RE = /(-?\d+(?:\.\d+)?)(ns|µs|us|ms|s|m|h)/g
const SINGLE_UNIT_RE = /^(-?\d+(?:\.\d+)?)(ns|µs|us|ms|s|m|h)$/

// parseGoDuration turns a Go `time.Duration` string into an
// amount+unit pair the number input and unit select can show. A plain
// single-unit value round-trips exactly ("90s" -> 90 seconds); anything
// else (a compound duration, or a sub-second unit outside this
// component's three choices) collapses to its total in seconds, since
// that is the finest unit this component offers.
export function parseGoDuration(
  value: string,
): { amount: string; unit: DurationUnit } | null {
  const trimmed = value.trim()
  if (!trimmed) {
    return null
  }

  const single = SINGLE_UNIT_RE.exec(trimmed)
  if (single) {
    const amount = single[1]!
    const suffix = single[2]!
    if (suffix === 's' || suffix === 'm' || suffix === 'h') {
      const unit: DurationUnit =
        suffix === 's' ? 'seconds' : suffix === 'm' ? 'minutes' : 'hours'
      return { amount, unit }
    }
    const seconds = Number(amount) * SUFFIX_TO_SECONDS[suffix]!
    return { amount: String(seconds), unit: 'seconds' }
  }

  const terms = [...trimmed.matchAll(DURATION_TERM_RE)]
  if (terms.length === 0) {
    return null
  }
  const totalSeconds = terms.reduce(
    (sum, term) => sum + Number(term[1]!) * SUFFIX_TO_SECONDS[term[2]!]!,
    0,
  )
  if (totalSeconds !== 0 && totalSeconds % 3600 === 0) {
    return { amount: String(totalSeconds / 3600), unit: 'hours' }
  }
  if (totalSeconds !== 0 && totalSeconds % 60 === 0) {
    return { amount: String(totalSeconds / 60), unit: 'minutes' }
  }
  return { amount: String(totalSeconds), unit: 'seconds' }
}

// composeGoDuration is parseGoDuration's inverse for the simple case
// this component always writes: a bare amount plus one of its three
// units, e.g. (5, "minutes") -> "5m". Blank/non-numeric amount composes
// to "" so an optional duration field can still be cleared.
export function composeGoDuration(amount: string, unit: DurationUnit): string {
  const trimmed = amount.trim()
  if (trimmed === '' || !Number.isFinite(Number(trimmed))) {
    return ''
  }
  return `${trimmed}${UNIT_SUFFIX[unit]}`
}

interface DurationInputProps {
  id?: string
  value: string
  onChange: (value: string) => void
  onBlur?: () => void
  placeholder?: string
  disabled?: boolean
}

// A number input paired with a seconds/minutes/hours unit select,
// composing to and parsing from the Go duration string the underlying
// form field (and the API) actually stores, so an operator never has to
// hand-type "2m" or "30s" themselves. Purely a controlled-input UX
// wrapper: `value`/`onChange` still carry the Go duration string, so
// nothing downstream changes.
export function DurationInput({
  id,
  value,
  onChange,
  onBlur,
  placeholder,
  disabled,
}: DurationInputProps) {
  const parsed = parseGoDuration(value)
  const [amount, setAmount] = useState(parsed?.amount ?? '')
  const [unit, setUnit] = useState<DurationUnit>(parsed?.unit ?? 'minutes')
  const lastEmitted = useRef(value)

  useEffect(() => {
    if (value === lastEmitted.current) {
      return
    }
    lastEmitted.current = value
    const next = parseGoDuration(value)
    setAmount(next?.amount ?? '')
    setUnit(next?.unit ?? 'minutes')
  }, [value])

  function emit(nextAmount: string, nextUnit: DurationUnit) {
    const composed = composeGoDuration(nextAmount, nextUnit)
    lastEmitted.current = composed
    onChange(composed)
  }

  return (
    <div className="flex gap-2">
      <Input
        id={id}
        type="number"
        step="any"
        min="0"
        placeholder={placeholder}
        value={amount}
        disabled={disabled}
        onChange={(e) => {
          setAmount(e.target.value)
          emit(e.target.value, unit)
        }}
        onBlur={onBlur}
      />
      <Select
        items={UNIT_LABEL}
        value={unit}
        onValueChange={(next) => {
          const nextUnit = next as DurationUnit
          setUnit(nextUnit)
          emit(amount, nextUnit)
        }}
      >
        <SelectTrigger
          id={id ? `${id}-unit` : undefined}
          className="w-32 shrink-0"
          disabled={disabled}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {(Object.keys(UNIT_LABEL) as DurationUnit[]).map((option) => (
            <SelectItem key={option} value={option}>
              {UNIT_LABEL[option]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}
