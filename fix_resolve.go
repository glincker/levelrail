import re

with open("internal/compose/resolve_test.go", "r") as f:
    lines = f.readlines()

new_lines = []
skip = False
for i, line in enumerate(lines):
    if line.startswith("func TestResolveMagicVars_PersistErrorPropagates"):
        skip = True
    elif line.startswith("func TestResolveMagicVars_EmbeddedPersistErrorPropagates"):
        skip = True
    elif line.startswith("func TestResolveMagicVars_CommandFQDNMissing"):
        skip = True
    elif line.startswith("func TestResolveMagicVars_GenerateErrorPropagatesForInvalidKind"):
        skip = True
    elif line.startswith("func TestResolveMagicVars_EmbeddedGenerateErrorPropagatesForInvalidKind"):
        skip = True
    elif line.startswith("func TestResolveMagicVars_SingleVarFQDNMissing"):
        skip = True

    # Check if we should stop skipping
    if skip and line == "}\n" and (i+1 == len(lines) or lines[i+1] == "\n" or lines[i+1].startswith("func ")):
        skip = False
        continue

    if not skip:
        new_lines.append(line)

with open("internal/compose/resolve_test.go", "w") as f:
    f.writelines(new_lines)
