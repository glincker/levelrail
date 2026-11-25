# Real-host proofs

`scripts/host-proof/` runs ssh-based proofs against real hosts. `scripts/proof/` is the PR proof pack and stays separate.

Usage: `PROOF_HOST=user@host PROOF_CONFIRM=yes [PROOF_LIVE=1] scripts/host-proof/run.sh <item>`.
Without `PROOF_LIVE=1` checks are only printed (dry run, no verdict).
Each live check prints PASS or FAIL with command, exit code and last output line. Any FAIL exits 1.
Items are `items/<name>.sh` defining `prove_<name>`; checks are `proof_check "<label>" <argv...>`.
Labels starting with `TODO` never run. Token-shaped values are masked in output.
