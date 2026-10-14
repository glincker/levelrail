# Shared settings, pulled in by every scene via Source.
Set Shell "bash"
Set FontSize 16
Set Width 1000
Set Height 520
Set Padding 20
Set Margin 0
Set BorderRadius 8
Set TypingSpeed 45ms
Set Framerate 20
Set WindowBar Colorful
Set Theme { "name": "demo", "black": "#0b1c22", "red": "#f0716b", "green": "#5fd3a0", "yellow": "#e8c26a", "blue": "#2fb3dc", "magenta": "#58b1ce", "cyan": "#2fb3dc", "white": "#d5e4ea", "brightBlack": "#5d7681", "brightRed": "#f0716b", "brightGreen": "#5fd3a0", "brightYellow": "#e8c26a", "brightBlue": "#58b1ce", "brightMagenta": "#58b1ce", "brightCyan": "#2fb3dc", "brightWhite": "#ffffff", "background": "#0b1c22", "foreground": "#d5e4ea", "selection": "#16414f", "cursor": "#2fb3dc" }

Hide
Type "export PATH=$PWD/scripts/demos/bin:$PATH PS1='$ ' DEMO_PACE=0.4" Enter
Type "curl() { levelrail-cli __install; }; sudo() { cat > /dev/null; levelrail-cli __install; }" Enter
Type "clear" Enter
Show
