const KIB = 1024

export type ByteUnit = 'b' | 'kib' | 'mib' | 'gib'

/** splitBytes scales a byte count to the largest unit that keeps it >= 1. */
export function splitBytes(bytes: number): { n: string; unit: ByteUnit } {
  if (bytes < KIB) return { n: String(Math.round(bytes)), unit: 'b' }
  if (bytes < KIB ** 2) return { n: (bytes / KIB).toFixed(1), unit: 'kib' }
  if (bytes < KIB ** 3) return { n: (bytes / KIB ** 2).toFixed(1), unit: 'mib' }
  return { n: (bytes / KIB ** 3).toFixed(1), unit: 'gib' }
}
