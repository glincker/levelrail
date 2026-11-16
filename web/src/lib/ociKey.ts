// Browser-side OCI API key helpers. Nothing here touches storage or the network.

const RSA_BITS = 2048

// OCI shows the fingerprint as an MD5 of the DER public key, which WebCrypto
// does not offer, so a small pure implementation is used.
const S = [
  7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 7, 12, 17, 22, 5, 9, 14, 20, 5,
  9, 14, 20, 5, 9, 14, 20, 5, 9, 14, 20, 4, 11, 16, 23, 4, 11, 16, 23, 4, 11,
  16, 23, 4, 11, 16, 23, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15, 21, 6, 10, 15,
  21,
]
const K = Array.from({ length: 64 }, (_, i) =>
  Math.floor(Math.abs(Math.sin(i + 1)) * 2 ** 32),
)

export function md5(data: Uint8Array): Uint8Array {
  const padded = Math.ceil((data.length + 9) / 64) * 64
  const buf = new Uint8Array(padded)
  buf.set(data)
  buf[data.length] = 0x80
  const view = new DataView(buf.buffer)
  view.setUint32(padded - 8, (data.length * 8) >>> 0, true)
  view.setUint32(padded - 4, Math.floor((data.length * 8) / 2 ** 32), true)
  let a0 = 0x67452301
  let b0 = 0xefcdab89
  let c0 = 0x98badcfe
  let d0 = 0x10325476
  for (let off = 0; off < padded; off += 64) {
    let a = a0
    let b = b0
    let c = c0
    let d = d0
    for (let i = 0; i < 64; i++) {
      let f: number
      let g: number
      if (i < 16) {
        f = (b & c) | (~b & d)
        g = i
      } else if (i < 32) {
        f = (d & b) | (~d & c)
        g = (5 * i + 1) % 16
      } else if (i < 48) {
        f = b ^ c ^ d
        g = (3 * i + 5) % 16
      } else {
        f = c ^ (b | ~d)
        g = (7 * i) % 16
      }
      const shift = S[i] ?? 0
      const sum =
        (a + f + (K[i] ?? 0) + view.getUint32(off + g * 4, true)) >>> 0
      a = d
      d = c
      c = b
      b = (b + ((sum << shift) | (sum >>> (32 - shift)))) >>> 0
    }
    a0 = (a0 + a) >>> 0
    b0 = (b0 + b) >>> 0
    c0 = (c0 + c) >>> 0
    d0 = (d0 + d) >>> 0
  }
  const out = new Uint8Array(16)
  const ov = new DataView(out.buffer)
  ;[a0, b0, c0, d0].forEach((v, i) => ov.setUint32(i * 4, v, true))
  return out
}

export function ociFingerprint(publicKeyDer: Uint8Array): string {
  return Array.from(md5(publicKeyDer), (b) =>
    b.toString(16).padStart(2, '0'),
  ).join(':')
}

function toPem(label: string, der: ArrayBuffer): string {
  const b64 = btoa(String.fromCharCode(...new Uint8Array(der)))
  const lines = b64.match(/.{1,64}/g) ?? []
  return `-----BEGIN ${label}-----\n${lines.join('\n')}\n-----END ${label}-----\n`
}

export interface OciKeyPair {
  publicKeyPem: string
  privateKeyPem: string
  fingerprint: string
}

export async function generateOciKeyPair(): Promise<OciKeyPair> {
  const pair = await crypto.subtle.generateKey(
    {
      name: 'RSASSA-PKCS1-v1_5',
      modulusLength: RSA_BITS,
      publicExponent: new Uint8Array([1, 0, 1]),
      hash: 'SHA-256',
    },
    true,
    ['sign', 'verify'],
  )
  const spki = await crypto.subtle.exportKey('spki', pair.publicKey)
  const pkcs8 = await crypto.subtle.exportKey('pkcs8', pair.privateKey)
  return {
    publicKeyPem: toPem('PUBLIC KEY', spki),
    privateKeyPem: toPem('PRIVATE KEY', pkcs8),
    fingerprint: ociFingerprint(new Uint8Array(spki)),
  }
}
