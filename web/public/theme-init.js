;(function () {
  try {
    var stored = localStorage.getItem('levelrail-theme')
    // Dark unless the visitor explicitly chose light or system.
    var isDark =
      stored === 'system'
        ? window.matchMedia('(prefers-color-scheme: dark)').matches
        : stored !== 'light'
    document.documentElement.classList.toggle('dark', isDark)
  } catch (e) {
    document.documentElement.classList.add('dark')
  }
})()
