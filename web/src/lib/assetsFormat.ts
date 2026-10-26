// Pure helpers for the Assets page: sizes, copy snippets and the upload threshold.

const DEFAULT_DIRECT_UPLOAD_BYTES = 4 * 1024 * 1024

// Bodies at or under this go through the API; larger ones use a signed URL.
export function directUploadLimit(): number {
  const raw = Number(
    (import.meta.env as Record<string, string | undefined>)
      .VITE_ASSETS_API_UPLOAD_MAX_BYTES,
  )
  return Number.isFinite(raw) && raw > 0 ? raw : DEFAULT_DIRECT_UPLOAD_BYTES
}

export function formatAssetSize(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let value = bytes
  let i = 0
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024
    i += 1
  }
  return `${value.toFixed(i > 0 && value < 10 ? 1 : 0)} ${units[i]}`
}

export function formatUsd(n: number): string {
  return `$${n.toFixed(n > 0 && n < 0.01 ? 4 : 2)}`
}

const IMAGE_EXT = /\.(png|jpe?g|gif|webp|avif|svg|bmp|ico)$/i

export function isImageAsset(key: string, contentType?: string): boolean {
  if (contentType?.startsWith('image/')) return true
  return IMAGE_EXT.test(key)
}

export function baseName(key: string): string {
  const i = key.lastIndexOf('/')
  return i >= 0 ? key.slice(i + 1) : key
}

export type CopyFormat = 'url' | 'markdown' | 'img'

export function copySnippet(
  format: CopyFormat,
  url: string,
  name: string,
): string {
  if (format === 'markdown') return `![${name}](${url})`
  if (format === 'img') return `<img src="${url}" alt="${name}" />`
  return url
}

export function corsRuleJson(rule: unknown): string {
  return JSON.stringify([rule], null, 2)
}

// "at least" is shown whenever the bounded listing hit its cap.
export function sizeLabel(
  bytes: number,
  atLeast: boolean,
  atLeastText: (size: string) => string,
): string {
  const size = formatAssetSize(bytes)
  return atLeast ? atLeastText(size) : size
}
