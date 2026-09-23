import sys

def check_dupes(filename):
    with open(filename, 'r') as f:
        lines = f.readlines()

    for i in range(len(lines)):
        if "func TestResolveMagicVars_EmbeddedGenerateErrorPropagates(" in lines[i]:
             print(f"TestResolveMagicVars_EmbeddedGenerateErrorPropagates found at line {i+1}")
        elif "func TestResolveMagicVars_PersistErrorPropagates(" in lines[i]:
             print(f"TestResolveMagicVars_PersistErrorPropagates found at line {i+1}")
        elif "func TestResolveMagicVars_EmbeddedPersistErrorPropagates(" in lines[i]:
             print(f"TestResolveMagicVars_EmbeddedPersistErrorPropagates found at line {i+1}")
        elif "func TestUnresolvedVar_String(" in lines[i]:
             print(f"TestUnresolvedVar_String found at line {i+1}")
        elif "func TestResolveMagicVars_CommandFQDNMissing(" in lines[i]:
             print(f"TestResolveMagicVars_CommandFQDNMissing found at line {i+1}")
        elif "func TestResolveMagicVars_SingleVarFQDNMissing(" in lines[i]:
             print(f"TestResolveMagicVars_SingleVarFQDNMissing found at line {i+1}")
        elif "func TestResolveMagicVars_GenerateErrorPropagatesForInvalidKind(" in lines[i]:
             print(f"TestResolveMagicVars_GenerateErrorPropagatesForInvalidKind found at line {i+1}")
        elif "func TestResolveMagicVars_EmbeddedGenerateErrorPropagatesForInvalidKind(" in lines[i]:
             print(f"TestResolveMagicVars_EmbeddedGenerateErrorPropagatesForInvalidKind found at line {i+1}")
        elif "func TestResolveMagicVars_NoVars(" in lines[i]:
             print(f"TestResolveMagicVars_NoVars found at line {i+1}")

check_dupes("internal/compose/resolve_test.go")
