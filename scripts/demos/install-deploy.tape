Source scripts/demos/lib.tape
Output docs/assets/demos/install-deploy.gif
Sleep 600ms
Type "curl -fsSL https://levelrail.com/install.sh | sudo sh" Enter
Sleep 3s
Type "levelrail-cli deploy myapp --image registry.example.com/acme/myapp:2.4.0" Enter
Sleep 6s
