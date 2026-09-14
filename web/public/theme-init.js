;(function () {
  try {
    var stored = localStorage.getItem('levelrail-theme')
    var isDark =
      stored === 'dark' ||
      (stored !== 'light' &&
        window.matchMedia('(prefers-color-scheme: dark)').matches)
    document.documentElement.classList.toggle('dark', isDark)
  } catch (e) {
    // localStorage/matchMedia can throw in private-browsing or
    // storage-restricted contexts. Leaving <html> without the
    // .dark class just means this visitor sees light mode, the
    // same safe default ThemeProvider's own try/catch falls back
    // to.
  }
})()
