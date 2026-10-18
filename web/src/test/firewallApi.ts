/** Routes fetch calls to canned JSON by "METHOD /path" prefix; unknown calls fail the test loudly. */
export function mockFirewallApi(routes: Record<string, unknown>) {
  const calls: { method: string; path: string; body?: string }[] = []
  const fn = (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.href
          : input.url
    const method = init?.method ?? 'GET'
    calls.push({ method, path: url, body: init?.body as string | undefined })
    const key = Object.keys(routes)
      .filter((k) => `${method} ${url}`.startsWith(k))
      .sort((a, b) => b.length - a.length)[0]
    if (!key) throw new Error(`unmocked request: ${method} ${url}`)
    const value = routes[key]
    if (typeof value === 'function') {
      return Promise.resolve((value as (i?: RequestInit) => Response)(init))
    }
    return Promise.resolve(jsonResponse(value))
  }
  return { fn, calls }
}

export function jsonResponse(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as Response
}
