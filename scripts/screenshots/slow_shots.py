#!/usr/bin/env python3
"""Capture shots.yml entries marked `loader: playwright` without waiting for the load event.

Some control-plane pages keep a request open long enough that shot-scraper's
default load wait times out. Entries with `loader: playwright` are left out
of the shot-scraper config (`split`) and captured here (`run`) with
domcontentloaded, a networkidle wait with a timeout, and the page's console
and network errors written to a log, so a page that never settles can be
reported as a possible bug.

Usage:
    slow_shots.py split <config_in> <config_out>
    slow_shots.py run <config> <storage_state.json> <log_path>
"""
import os
import re
import sys

from playwright.sync_api import sync_playwright

BLOCK_START = re.compile(r"(?m)^(?=- output: )")
PASSKEY_OUTPUT = "docs/assets/screenshots/account-passkeys.png"
PASSKEY_NAMES = ["MacBook Touch ID", "iPhone"]


def register_passkeys(context, page, log):
    """Register two passkeys through the real Add dialog, using a CDP virtual authenticator."""
    import json

    with open(os.path.join(os.getcwd(), "web/src/locales/en/passkeys.json")) as f:
        copy = json.load(f)
    cdp = context.new_cdp_session(page)
    cdp.send("WebAuthn.enable", {"enableUI": False})
    cdp.send("WebAuthn.addVirtualAuthenticator", {"options": {
        "protocol": "ctap2", "transport": "internal", "hasResidentKey": True,
        "hasUserVerification": True, "isUserVerified": True, "automaticPresenceSimulation": True}})
    done = 0
    for name in PASSKEY_NAMES:
        try:
            trigger = page.get_by_role("button", name=copy["card"]["add"]).first
            if trigger.count() == 0:
                trigger = page.get_by_role("button", name=copy["empty"]["action"]).first
            trigger.click(timeout=15000)
            dialog = page.get_by_role("dialog").first
            dialog.wait_for(timeout=10000)
            dialog.get_by_label(copy["addDialog"]["nameLabel"]).fill(name)
            dialog.get_by_role("button", name=copy["addDialog"]["continue"]).click()
            dialog.get_by_text(copy["addDialog"]["savedTitle"]).wait_for(timeout=30000)
            dialog.get_by_role("button", name=copy["addDialog"]["done"]).click()
            page.wait_for_timeout(1500)
            done += 1
        except Exception as exc:  # noqa: BLE001 - report the UI step that failed
            log.write(f"    passkey registration stopped at {name!r}: {exc}\n")
            page.keyboard.press("Escape")
            break
    log.write(f"    passkeys registered through the UI: {done} of {len(PASSKEY_NAMES)}\n")
    return done


def parse_blocks(text):
    """Return (header, [entry dict, ...]) for a shots.yml-style config."""
    chunks = BLOCK_START.split(text)
    header, entries = "", []
    for chunk in chunks:
        if not chunk.startswith("- output: "):
            header += chunk
            continue
        entry, raw = {}, chunk
        for line in raw.splitlines():
            stripped = line.strip()
            if stripped.startswith("- output:"):
                key, value = "output", stripped[len("- output:"):].strip()
            elif ":" in stripped:
                key, value = stripped.split(":", 1)
                key, value = key.strip(), value.strip()
            else:
                continue
            if len(value) >= 2 and value[0] == value[-1] == '"':
                value = value[1:-1]
            entry[key] = value
        entry["_raw"] = raw
        entries.append(entry)
    return header, entries


def split(config_in, config_out):
    with open(config_in) as f:
        header, entries = parse_blocks(f.read())
    kept = [e["_raw"] for e in entries if e.get("loader") != "playwright"]
    with open(config_out, "w") as f:
        f.write(header + "".join(kept))


def run(config, state_path, log_path):
    with open(config) as f:
        _, entries = parse_blocks(f.read())
    entries = [e for e in entries if e.get("loader") == "playwright"]
    if not entries:
        return 0
    failures = 0
    with sync_playwright() as p, open(log_path, "a") as log:
        browser = p.chromium.launch()
        context = browser.new_context(storage_state=state_path)
        for entry in entries:
            page = context.new_page()
            page.set_viewport_size({"width": int(entry["width"]), "height": int(entry["height"])})
            problems = []
            page.on("console", lambda m: problems.append(f"console.{m.type}: {m.text[:200]}")
                    if m.type == "error" else None)
            page.on("requestfailed", lambda r: problems.append(f"requestfailed: {r.url[:160]}"))
            page.on("response", lambda r: problems.append(f"http {r.status}: {r.url[:160]}")
                    if r.status >= 400 else None)
            try:
                page.goto(entry["url"], wait_until="domcontentloaded", timeout=60000)
            except Exception as exc:  # noqa: BLE001 - report and keep going
                problems.append(f"goto: {exc}")
            settled = True
            try:
                page.wait_for_load_state("networkidle", timeout=20000)
            except Exception:  # noqa: BLE001
                settled = False
            if entry["output"] == PASSKEY_OUTPUT:
                register_passkeys(context, page, log)
            script = entry.get("javascript")
            if script:
                try:
                    page.evaluate(script)
                except Exception as exc:  # noqa: BLE001
                    problems.append(f"javascript: {exc}")
            page.wait_for_timeout(int(entry.get("wait", "2000")))
            output = entry["output"]
            os.makedirs(os.path.dirname(output) or ".", exist_ok=True)
            page.screenshot(path=output)
            unique = list(dict.fromkeys(problems))[:25]
            log.write(f"{output} url={entry['url']} networkidle={settled}\n")
            for line in unique:
                log.write(f"    {line}\n")
            if not settled:
                failures += 1
            page.close()
        browser.close()
    print(f"slow shots: {len(entries)} captured, {failures} never reached networkidle (see {log_path})")
    return 0


def main(argv):
    if len(argv) == 4 and argv[1] == "split":
        split(argv[2], argv[3])
        return 0
    if len(argv) == 5 and argv[1] == "run":
        return run(argv[2], argv[3], argv[4])
    print(__doc__)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
