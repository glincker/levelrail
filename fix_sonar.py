import re

with open("internal/compose/resolve_test.go", "r") as f:
    content = f.read()

# SonarCloud complains about duplication.
# 1. TestResolveMagicVars_CommandFQDNMissing is essentially the exact same structure as TestResolveMagicVars_Command
# 2. TestResolveMagicVars_PersistErrorPropagates and TestResolveMagicVars_EmbeddedPersistErrorPropagates are very similar.
# 3. TestResolveMagicVars_GenerateErrorPropagatesForInvalidKind is very similar to TestResolveMagicVars_GenerateErrorPropagates.

# Let's fix this by removing the duplicated functions and just adding their test logic to the existing ones where appropriate, or paramterizing.
