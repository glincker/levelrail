// Browser-side helpers for drop deploy: turn a dropped folder or zip into one zip body for the upload endpoint.

export interface SiteFile {
  /** Path relative to the site root, forward slashes. */
  path: string
  file: File
}

export type DropSelection =
  | { kind: 'zip'; file: File; label: string; bytes: number }
  | {
      kind: 'folder'
      files: SiteFile[]
      label: string
      bytes: number
      hasIndex: boolean
    }

/** Zip32 has no room past these; the server caps are far lower by default. */
export const ZIP_MAX_ENTRIES = 65535
export const ZIP_MAX_BYTES = 0xffffffff - 1024 * 1024

const IGNORED_SEGMENTS = new Set(['.git', 'node_modules', '__MACOSX'])
const IGNORED_FILES = new Set(['.DS_Store', 'Thumbs.db'])

export function isIgnoredPath(path: string): boolean {
  const parts = path.split('/')
  const last = parts[parts.length - 1] ?? ''
  return (
    IGNORED_FILES.has(last) ||
    parts.slice(0, -1).some((p) => IGNORED_SEGMENTS.has(p))
  )
}

export function isZipFile(file: File): boolean {
  return /\.zip$/i.test(file.name)
}

/** Valid app name from a folder or zip name: lowercase letters, digits and dashes. */
export function siteNameFrom(label: string): string {
  const base = label.replace(/\.zip$/i, '')
  return base
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)
}

function folderSelection(
  label: string,
  files: SiteFile[],
): DropSelection | null {
  const kept = files.filter((f) => !isIgnoredPath(f.path))
  if (kept.length === 0) return null
  return {
    kind: 'folder',
    files: kept,
    label,
    bytes: kept.reduce((n, f) => n + f.file.size, 0),
    hasIndex: kept.some((f) => f.path === 'index.html'),
  }
}

/** Selection from an <input type="file"> (zip or webkitdirectory). */
export function selectionFromFileList(
  list: FileList | File[],
): DropSelection | null {
  const files = Array.from(list)
  if (files.length === 0) return null
  const first = files[0]
  if (files.length === 1 && first && isZipFile(first)) {
    return { kind: 'zip', file: first, label: first.name, bytes: first.size }
  }
  const rel = (f: File) =>
    (f as File & { webkitRelativePath?: string }).webkitRelativePath || f.name
  const root = rel(first as File).includes('/')
    ? rel(first as File).split('/')[0]
    : ''
  const prefix = root ? `${root}/` : ''
  return folderSelection(
    root || 'site',
    files.map((file) => ({
      path: rel(file).startsWith(prefix)
        ? rel(file).slice(prefix.length)
        : rel(file),
      file,
    })),
  )
}

type FsEntry = FileSystemEntry

async function readAllEntries(
  dir: FileSystemDirectoryEntry,
): Promise<FsEntry[]> {
  const reader = dir.createReader()
  const out: FsEntry[] = []
  for (;;) {
    const batch = await new Promise<FsEntry[]>((resolve, reject) =>
      reader.readEntries(resolve, reject),
    )
    if (batch.length === 0) return out
    out.push(...batch)
  }
}

async function walkEntry(entry: FsEntry, prefix: string, out: SiteFile[]) {
  if (entry.isFile) {
    const file = await new Promise<File>((resolve, reject) =>
      (entry as FileSystemFileEntry).file(resolve, reject),
    )
    out.push({ path: `${prefix}${entry.name}`, file })
    return
  }
  if (IGNORED_SEGMENTS.has(entry.name)) return
  for (const child of await readAllEntries(entry as FileSystemDirectoryEntry)) {
    await walkEntry(child, `${prefix}${entry.name}/`, out)
  }
}

/** Selection from a drop event, walking dropped folders. */
export async function selectionFromDataTransfer(
  dt: DataTransfer,
): Promise<DropSelection | null> {
  const entries: FsEntry[] = []
  for (const item of Array.from(dt.items ?? [])) {
    const entry = item.kind === 'file' ? item.webkitGetAsEntry?.() : null
    if (entry) entries.push(entry)
  }
  if (entries.length === 0) return selectionFromFileList(dt.files)
  const only = entries.length === 1 ? entries[0] : undefined
  if (only?.isFile) return selectionFromFileList(dt.files)
  const files: SiteFile[] = []
  if (only?.isDirectory) {
    for (const child of await readAllEntries(
      only as FileSystemDirectoryEntry,
    )) {
      await walkEntry(child, '', files)
    }
    return folderSelection(only.name, files)
  }
  for (const e of entries) await walkEntry(e, '', files)
  return folderSelection('site', files)
}

let crcTable: Uint32Array | null = null

export function crc32(bytes: Uint8Array): number {
  if (!crcTable) {
    crcTable = new Uint32Array(256)
    for (let n = 0; n < 256; n++) {
      let c = n
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
      crcTable[n] = c >>> 0
    }
  }
  let crc = 0xffffffff
  for (let i = 0; i < bytes.length; i++) {
    crc =
      (crcTable[(crc ^ (bytes[i] as number)) & 0xff] as number) ^ (crc >>> 8)
  }
  return (crc ^ 0xffffffff) >>> 0
}

async function deflateRaw(bytes: Uint8Array): Promise<Uint8Array | null> {
  if (typeof CompressionStream === 'undefined' || bytes.length < 256)
    return null
  try {
    const stream = new Blob([bytes as BlobPart])
      .stream()
      .pipeThrough(new CompressionStream('deflate-raw'))
    const out = new Uint8Array(await new Response(stream).arrayBuffer())
    return out.length < bytes.length ? out : null
  } catch {
    return null
  }
}

/** Builds a zip (deflate when the browser supports it, else stored) from the files. */
export async function buildZip(files: SiteFile[]): Promise<Blob> {
  if (files.length > ZIP_MAX_ENTRIES) throw new Error('too-many-files')
  const enc = new TextEncoder()
  const parts: BlobPart[] = []
  const central: Uint8Array[] = []
  let offset = 0
  for (const { path, file } of files) {
    const raw = new Uint8Array(await file.arrayBuffer())
    const packed = await deflateRaw(raw)
    const data = packed ?? raw
    const name = enc.encode(path)
    const crc = crc32(raw)
    const method = packed ? 8 : 0

    const local = new DataView(new ArrayBuffer(30))
    local.setUint32(0, 0x04034b50, true)
    local.setUint16(4, 20, true)
    local.setUint16(6, 0x0800, true)
    local.setUint16(8, method, true)
    local.setUint32(14, crc, true)
    local.setUint32(18, data.length, true)
    local.setUint32(22, raw.length, true)
    local.setUint16(26, name.length, true)
    parts.push(local.buffer, name, data as BlobPart)

    const cd = new DataView(new ArrayBuffer(46))
    cd.setUint32(0, 0x02014b50, true)
    cd.setUint16(4, 20, true)
    cd.setUint16(6, 20, true)
    cd.setUint16(8, 0x0800, true)
    cd.setUint16(10, method, true)
    cd.setUint32(16, crc, true)
    cd.setUint32(20, data.length, true)
    cd.setUint32(24, raw.length, true)
    cd.setUint16(28, name.length, true)
    cd.setUint32(42, offset, true)
    central.push(new Uint8Array(cd.buffer), name)

    offset += 30 + name.length + data.length
    if (offset > ZIP_MAX_BYTES) throw new Error('too-large')
  }
  const cdSize = central.reduce((n, c) => n + c.length, 0)
  const end = new DataView(new ArrayBuffer(22))
  end.setUint32(0, 0x06054b50, true)
  end.setUint16(8, files.length, true)
  end.setUint16(10, files.length, true)
  end.setUint32(12, cdSize, true)
  end.setUint32(16, offset, true)
  return new Blob([...parts, ...(central as BlobPart[]), end.buffer], {
    type: 'application/zip',
  })
}
