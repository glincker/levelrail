(function () {
  var KEY = 'levelrail.server'
  var FORWARD = ['image', 'name', 'port', 'env', 'health', 'source']
  var form = document.getElementById('dl-form')
  var input = document.getElementById('dl-server')
  var errorEl = document.getElementById('dl-error')
  var saved = document.getElementById('dl-saved')
  var savedHost = document.getElementById('dl-saved-host')
  var go = document.getElementById('dl-go')
  var change = document.getElementById('dl-change')
  var summary = document.getElementById('dl-summary')

  function readStored() {
    try {
      return window.localStorage.getItem(KEY) || ''
    } catch (e) {
      return ''
    }
  }

  function store(v) {
    try {
      window.localStorage.setItem(KEY, v)
    } catch (e) {}
  }

  function clearStored() {
    try {
      window.localStorage.removeItem(KEY)
    } catch (e) {}
  }

  function normalize(raw) {
    var v = raw.trim()
    if (!/^https?:\/\//i.test(v)) v = 'https://' + v
    var u
    try {
      u = new URL(v)
    } catch (e) {
      return null
    }
    if (u.protocol !== 'https:' && u.protocol !== 'http:') return null
    if (u.username || u.password || !u.hostname) return null
    return u.origin
  }

  function query() {
    var incoming = new URLSearchParams(window.location.search)
    var out = new URLSearchParams()
    out.set('deploy', '1')
    FORWARD.forEach(function (k) {
      var v = incoming.get(k)
      if (v) out.set(k, v.slice(0, 2048))
    })
    return out
  }

  function target(origin) {
    return origin + '/apps/new?' + query().toString()
  }

  function describe() {
    var q = query()
    var parts = []
    ;['image', 'name', 'port'].forEach(function (k) {
      if (q.get(k)) parts.push(k + ': ' + q.get(k))
    })
    summary.textContent = parts.length ? parts.join(', ') : ''
  }

  function showSaved(origin) {
    savedHost.textContent = new URL(origin).host
    saved.hidden = false
    form.hidden = true
  }

  describe()
  var stored = normalize(readStored())
  if (stored) showSaved(stored)

  go.addEventListener('click', function () {
    var origin = normalize(readStored())
    if (origin) window.location.assign(target(origin))
  })

  change.addEventListener('click', function () {
    clearStored()
    saved.hidden = true
    form.hidden = false
    input.focus()
  })

  form.addEventListener('submit', function (e) {
    e.preventDefault()
    var origin = normalize(input.value)
    if (!origin) {
      errorEl.hidden = false
      return
    }
    errorEl.hidden = true
    store(origin)
    window.location.assign(target(origin))
  })
})()
