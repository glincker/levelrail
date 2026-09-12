#!/usr/bin/env python3
"""Log into a running Levelrail control plane through the real /login form
and dump the resulting session as a Playwright storage_state.json, so
shot-scraper can reuse it via --auth instead of every shot re-logging in.

Usage:
    python3 login_state.py <storage_state_path> [session_cookie_path]

The optional second path gets just the raw session_token cookie value
(no JSON), so a caller like capture.sh can reuse the same real login for
a plain curl call (e.g. approving a CLI device-login code) without
re-parsing the storage_state format.

Env vars:
    LEVELRAIL_URL     base URL of the running control plane (default http://localhost:8080)
    LEVELRAIL_USER    username (default "dev")
    LEVELRAIL_PASS    password (default "dev")
"""
import os
import sys

from playwright.sync_api import sync_playwright

BASE_URL = os.environ.get("LEVELRAIL_URL", "http://localhost:8080")
USERNAME = os.environ.get("LEVELRAIL_USER", "dev")
PASSWORD = os.environ.get("LEVELRAIL_PASS", "dev")
OUTPUT_PATH = sys.argv[1] if len(sys.argv) > 1 else "storage_state.json"
COOKIE_OUTPUT_PATH = sys.argv[2] if len(sys.argv) > 2 else None


def main() -> None:
    with sync_playwright() as p:
        browser = p.chromium.launch()
        page = browser.new_page()
        page.goto(f"{BASE_URL}/login")
        page.fill("#login-username", USERNAME)
        page.fill("#login-password", PASSWORD)
        page.click("button[type=submit]")
        page.wait_for_url(lambda url: "/login" not in url, timeout=15000)
        state = page.context.storage_state(path=OUTPUT_PATH)
        browser.close()

    if COOKIE_OUTPUT_PATH:
        session_cookie = next(
            (c["value"] for c in state["cookies"] if c["name"] == "session_token"),
            None,
        )
        if session_cookie is None:
            raise SystemExit("login succeeded but no session_token cookie was set")
        with open(COOKIE_OUTPUT_PATH, "w") as f:
            f.write(session_cookie)

    print(f"wrote session state to {OUTPUT_PATH}")


if __name__ == "__main__":
    main()
