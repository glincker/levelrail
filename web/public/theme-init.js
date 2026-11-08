;(function () {
  try {
    var stored = localStorage.getItem('levelrail-theme')
    // Dark unless the visitor explicitly chose light or system.
    var isDark =
      stored === 'system'
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
        : stored !== 'light'
    document.documentElement.classList.toggle(
      'dark',
      isDark || stored === 'oled',
    )
    document.documentElement.classList.toggle('oled', stored === 'oled')
    try {
      var saved = JSON.parse(localStorage.getItem('levelrail-appearance-v1'))
      if (saved && saved.look === 'restrained') {
        document.documentElement.setAttribute('data-look', 'restrained')
      }
    } catch (e) {}
    // document.write keeps the link parser-inserted, so first paint waits for it.
    if (stored === 'oled') {
      document.write(
        '<link rel="stylesheet" id="levelrail-oled-css" href="/glinr.oled.css">',
      )
    }
  } catch (e) {
    document.documentElement.classList.add('dark')
  }
})()
