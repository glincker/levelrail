import { describe, expect, it } from 'vitest'
import {
  buildZip,
  crc32,
  isIgnoredPath,
  selectionFromFileList,
  siteNameFrom,
  ZIP_MAX_ENTRIES,
} from './zipSite'

function fileAt(path: string, body = 'x'): File {
  const f = new File([body], path.split('/').pop() ?? path)
  Object.defineProperty(f, 'webkitRelativePath', { value: path })
  return f
}

describe('crc32', () => {
  it('matches the standard check value', () => {
    expect(crc32(new TextEncoder().encode('123456789'))).toBe(0xcbf43926)
  })
})

describe('siteNameFrom', () => {
  it.each([
    ['My Site.zip', 'my-site'],
    ['portfolio_v2', 'portfolio-v2'],
    ['--Docs!!--', 'docs'],
    ['', ''],
  ])('%s -> %s', (input, want) => {
    expect(siteNameFrom(input)).toBe(want)
  })
})

describe('isIgnoredPath', () => {
  it.each([
    ['index.html', false],
    ['css/a.css', false],
    ['.git/config', true],
    ['node_modules/x/index.js', true],
    ['a/.DS_Store', true],
    ['__MACOSX/a', true],
  ])('%s -> %s', (path, want) => {
    expect(isIgnoredPath(path)).toBe(want)
  })
})

describe('selectionFromFileList', () => {
  it('returns null for an empty list', () => {
    expect(selectionFromFileList([])).toBeNull()
  })

  it('treats a single .zip as a zip', () => {
    const sel = selectionFromFileList([new File(['PK'], 'Site.ZIP')])
    expect(sel).toMatchObject({ kind: 'zip', label: 'Site.ZIP' })
  })

  it('strips the dropped folder name and detects index.html', () => {
    const sel = selectionFromFileList([
      fileAt('mysite/index.html'),
      fileAt('mysite/css/a.css'),
      fileAt('mysite/.git/HEAD'),
    ])
    expect(sel).toMatchObject({
      kind: 'folder',
      label: 'mysite',
      hasIndex: true,
    })
    expect(sel?.kind === 'folder' && sel.files.map((f) => f.path)).toEqual([
      'index.html',
      'css/a.css',
    ])
  })

  it('flags a folder with no index.html at its top', () => {
    const sel = selectionFromFileList([fileAt('dist/assets/a.js')])
    expect(sel).toMatchObject({ kind: 'folder', hasIndex: false })
  })

  it('returns null when everything is ignored', () => {
    expect(selectionFromFileList([fileAt('s/node_modules/a.js')])).toBeNull()
  })
})

describe('buildZip', () => {
  it('writes local headers, a central directory and an end record', async () => {
    const blob = await buildZip([
      { path: 'index.html', file: new File(['hi'], 'index.html') },
      { path: 'big.txt', file: new File(['a'.repeat(2000)], 'big.txt') },
    ])
    const buf = new DataView(await blob.arrayBuffer())
    expect(buf.getUint32(0, true)).toBe(0x04034b50)
    const end = buf.byteLength - 22
    expect(buf.getUint32(end, true)).toBe(0x06054b50)
    expect(buf.getUint16(end + 10, true)).toBe(2)
    const cdOffset = buf.getUint32(end + 16, true)
    expect(buf.getUint32(cdOffset, true)).toBe(0x02014b50)
    expect(buf.getUint32(cdOffset + 16, true)).toBe(
      crc32(new TextEncoder().encode('hi')),
    )
  })

  it('refuses more entries than a zip can index', async () => {
    const files = Array.from({ length: ZIP_MAX_ENTRIES + 1 }, (_, i) => ({
      path: `f${i}`,
      file: new File([], `f${i}`),
    }))
    await expect(buildZip(files)).rejects.toThrow('too-many-files')
  })
})
