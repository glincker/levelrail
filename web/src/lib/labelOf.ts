// Backend values can outrun this build's maps, so a lookup falls back to the raw key.
export function labelOf(
  map: Readonly<Record<string, string>>,
  key: string,
): string {
  return Object.hasOwn(map, key) ? (map[key] ?? key) : key
}

// For link maps: an unknown key yields no URL, never a broken link.
export function linkOf(
  map: Readonly<Record<string, string>>,
  key: string,
): string | undefined {
  return Object.hasOwn(map, key) ? map[key] : undefined
}
