const SAFE_NAME = /^[A-Za-z0-9._-]+$/

export function shellQuote(value: string): string {
  return SAFE_NAME.test(value) ? value : `'${value.replace(/'/g, `'\\''`)}'`
}
