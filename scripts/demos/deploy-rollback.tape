Source scripts/demos/lib.tape
Output docs/assets/demos/deploy-rollback.gif
Sleep 600ms
Type "levelrail-cli deploy myapp --image registry.example.com/acme/myapp:2.4.0" Enter
Sleep 4s
Type "# 2.4.0 throws errors in production, go back" Enter
Sleep 800ms
Type "levelrail-cli rollback myapp --image registry.example.com/acme/myapp:2.3.1" Enter
Sleep 4s
