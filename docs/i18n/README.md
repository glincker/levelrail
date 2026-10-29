# Translated READMEs

`docs/i18n/README.<lang>.md` are AI-assisted translations of the root `README.md`. They are kept in sync by `scripts/i18n/readme-sync.sh`, which needs no CI API key: the model step runs locally through the `claude` CLI.

## How it works

- **stamp**: line 1 of each translation is `<!-- translated-from: README.md blob:<hash> lang:<lang> -->`. The hash is the git blob id of the English README with the language nav block (`<!-- i18n-nav:start -->` to `<!-- i18n-nav:end -->`) removed, so adding a language never makes the others stale. Stamping also writes that English text into the git object store.
- **check**: `scripts/i18n/readme-sync.sh check` lists each translation as `fresh`, `STALE` or `MISSING stamp`. It exits 0 unless you pass `--strict`.
- **validate**: `scripts/i18n/readme-sync.sh validate <lang>` compares structure with the English README: heading count per level, byte-identical fenced code blocks, and identical link and image URL lists (link text ignored, `#fragment` links and a leading `../` ignored). It exits 1 with a diff summary on mismatch.
- **update**: `scripts/i18n/readme-sync.sh update [lang...]` (default: all stale) sends the glossary, the old English, the new English and the current translation to `claude -p --model sonnet`. The model is told to change only passages whose English changed and to keep every other sentence, including human corrections, byte-for-byte. The result is validated, and only then replaces the file and is re-stamped. On failure the old file stays and the reason is printed. Exit code 2 means `claude` is not on PATH.

The old English comes from the stamped blob. Recover it with `git cat-file -p <blob>` while the repo still has that object. If the blob is missing (for example a fresh shallow clone), `update` falls back to a full retranslation.

The pre-commit hook prints a non-failing warning when you stage `README.md` and any translation is stale.

## Fix a translation

1. Edit `docs/i18n/README.<lang>.md` directly.
2. Run `scripts/i18n/readme-sync.sh validate <lang>`. Do not touch code blocks, URLs or badge images.
3. Open a PR. The next `update` preserves your wording for sentences whose English did not change.

## Add a language

1. Create `docs/i18n/README.<lang>.md` (BCP 47 tag, for example `ko` or `zh-Hant`) from the English README, using `glossary.md`.
2. Put this note right under the title:

   > This translation was generated with AI and may contain mistakes. Corrections are welcome: see [docs/i18n/README.md](README.md).

3. Add the language to the nav block in `README.md` and to `glossary.md`.
4. Run `validate <lang>`, then `stamp <lang>`, and open a PR.

## Optional workflow

Not installed by default. Save as `.github/workflows/i18n-readme.yml` to warn on pull requests:

```yaml
name: i18n-readme
on:
  pull_request:
    paths: ["README.md", "docs/i18n/**"]
jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Warn on stale translations
        run: |
          out=$(scripts/i18n/readme-sync.sh check)
          echo "$out"
          if echo "$out" | grep -qE 'STALE|MISSING'; then
            echo "::warning::translated READMEs are stale: run scripts/i18n/readme-sync.sh update"
          fi
```
