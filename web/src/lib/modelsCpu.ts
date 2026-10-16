import type { Tone } from '@/components/kit'
import type {
  CpuFitVerdict,
  LocalModelCard,
  LocalModelTier,
} from '../types/modelsCpu'

const GIB = 1024 ** 3

export function formatGiB(bytes: number): string {
  const gib = bytes / GIB
  return `${gib >= 10 ? gib.toFixed(0) : gib.toFixed(1)} GB`
}

export function formatMemoryMiB(mib: number): string {
  return formatGiB(mib * 1024 * 1024)
}

export function formatCpus(milli: number): string {
  const cpus = milli / 1000
  return Number.isInteger(cpus) ? String(cpus) : cpus.toFixed(1)
}

export function verdictTone(verdict: CpuFitVerdict): Tone {
  switch (verdict) {
    case 'fits_comfortably':
      return 'success'
    case 'tight':
      return 'warning'
    case 'wont_fit':
      return 'danger'
    default:
      return 'neutral'
  }
}

export function speedRange(lo: number, hi: number): string {
  if (hi <= 0) return ''
  return `${lo.toFixed(1)} to ${hi.toFixed(1)}`
}

const TIER_ORDER: Record<LocalModelTier, number> = {
  light: 0,
  default: 1,
  plus: 2,
}

export function sortCards(cards: LocalModelCard[]): LocalModelCard[] {
  return [...cards].sort(
    (a, b) =>
      TIER_ORDER[a.tier] - TIER_ORDER[b.tier] ||
      a.download_bytes - b.download_bytes,
  )
}

// Cards for the vendors thesvg has a mark for use a template logo id;
// others (Nomic) fall back to the monogram tile.
const VENDOR_LOGO: Record<string, string> = {
  ibm: 'local-granite',
  qwen: 'local-qwen',
  google: 'local-gemma',
}

export function vendorLogoId(vendor: string): string | undefined {
  return VENDOR_LOGO[vendor]
}

export function suggestedName(catalogId: string): string {
  return catalogId.replace(/\./g, '-').slice(0, 42)
}

export function clamp(n: number, lo: number, hi: number): number {
  return Math.min(Math.max(n, lo), hi)
}

export function downloadPercent(done: number, total: number): number {
  if (total <= 0) return 0
  return clamp(Math.round((done / total) * 100), 0, 100)
}
