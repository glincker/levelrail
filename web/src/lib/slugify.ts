// Approximates VitePress/GitHub heading-anchor slugs closely enough for
// this docs set's plain ASCII headings. Mirrored (not imported, Node
// script vs browser bundle) by scripts/build-docs-manifest.mjs, which
// must stay byte-for-byte identical or generated anchors stop matching
// rendered ones.
export function slugify(text: string): string {
  return text
    .toLowerCase()
    .trim()
    .replace(/[`~!@#$%^&*()+={}[\]|\\:;"'<>,.?/]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
}

// Appends -1, -2, ... on repeat slugs within one page, the same
// disambiguation GitHub/markdown-it-anchor apply, tracked per call site
// via the caller-supplied `seen` map.
export function uniqueSlug(text: string, seen: Map<string, number>): string {
  const base = slugify(text)
  const count = seen.get(base) ?? 0
  seen.set(base, count + 1)
  return count === 0 ? base : `${base}-${count}`
}
