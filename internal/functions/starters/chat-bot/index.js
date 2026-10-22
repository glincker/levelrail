'use strict'
const http = require('node:http')
const { createKV } = require('./kv')
const { createHandler } = require('./handler')

const handle = createHandler({ env: process.env, kv: createKV(process.env), fetch: globalThis.fetch })
const MAX_BODY = Number(process.env.MAX_BODY_BYTES || 1048576)

function send(res, out) {
  const headers = { ...(out.headers || {}) }
  let body = out.body
  if (body !== undefined && typeof body === 'object' && !Buffer.isBuffer(body)) {
    headers['content-type'] = 'application/json'
    body = JSON.stringify(body)
  }
  res.writeHead(out.status || 200, headers).end(body)
}

const server = http.createServer((req, res) => {
  const chunks = []
  let size = 0
  let tooLarge = false
  req.on('data', (c) => {
    size += c.length
    if (size > MAX_BODY) tooLarge = true
    else chunks.push(c)
  })
  req.on('end', async () => {
    if (tooLarge) return send(res, { status: 413, body: { error: 'payload too large' } })
    const url = new URL(req.url, 'http://localhost')
    try {
      const out = await handle({ method: req.method, path: url.pathname, query: url.searchParams, headers: req.headers, body: Buffer.concat(chunks) })
      send(res, out)
    } catch (err) {
      process.stderr.write(`handler error: ${err.message}\n`)
      send(res, { status: 500, body: { error: 'internal error' } })
    }
  })
})

server.listen(Number(process.env.PORT || 8080))
