import re

with open("internal/compose/resolve_test.go", "r") as f:
    content = f.read()

# We need to remove the first block of functions that were accidentally added twice.
# Or wait, what happened? Let's check what's currently in the file.
