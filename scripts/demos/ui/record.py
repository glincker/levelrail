#!/usr/bin/env python3
"""Record two dashboard scenes as video against a running, seeded control plane.

Usage:
    LEVELRAIL_URL=http://localhost:8098 python3 record.py <out_dir>

Seed the server first: scripts/screenshots/capture.sh deploys the
marketing-site app with three deploys and metrics, and keeps it only for
its own run, so run this script from a copy of that pipeline or against
any server that already has an app named by DEMO_APP with 2+ deploys.

Env vars: LEVELRAIL_URL, LEVELRAIL_USER, LEVELRAIL_PASS (same as
login_state.py), DEMO_APP (default marketing-site).
Writes <out_dir>/deploy-live.webm and <out_dir>/rollback.webm.
"""
import os
import re
import sys

from playwright.sync_api import Page, sync_playwright

BASE_URL = os.environ.get("LEVELRAIL_URL", "http://localhost:8080")
USERNAME = os.environ.get("LEVELRAIL_USER", "dev")
PASSWORD = os.environ.get("LEVELRAIL_PASS", "dev")
APP = os.environ.get("DEMO_APP", "marketing-site")
OUT_DIR = sys.argv[1] if len(sys.argv) > 1 else "."
SIZE = {"width": 1280, "height": 760}


def login(page: Page) -> None:
    page.goto(f"{BASE_URL}/login")
    page.fill("#login-username", USERNAME)
    page.get_by_role("button", name="Continue").click()
    page.wait_for_selector("#login-password", timeout=15000)
    page.fill("#login-password", PASSWORD)
    page.click("button[type=submit]")
    page.wait_for_url(lambda url: "/login" not in url, timeout=15000)


def scene(browser, name: str, body) -> None:
    ctx = browser.new_context(
        viewport=SIZE,
        record_video_dir=OUT_DIR,
        record_video_size=SIZE,
        color_scheme="dark",
    )
    page = ctx.new_page()
    login(page)
    body(page)
    video = page.video
    ctx.close()
    video.save_as(os.path.join(OUT_DIR, f"{name}.webm"))


def deploy_live(page: Page) -> None:
    page.goto(f"{BASE_URL}/apps/{APP}/metrics")
    page.wait_for_timeout(2500)
    page.get_by_role("button", name=re.compile(r"redeploy", re.I)).first.click()
    confirm = page.get_by_role("button", name=re.compile(r"^(confirm|redeploy)", re.I))
    if confirm.count():
        confirm.last.click()
    page.wait_for_timeout(6000)
    page.mouse.move(640, 380)
    page.wait_for_timeout(2500)


def rollback(page: Page) -> None:
    page.goto(f"{BASE_URL}/apps/{APP}/deploys")
    page.wait_for_timeout(2500)
    page.get_by_role("button", name=re.compile(r"roll ?back", re.I)).first.click()
    confirm = page.get_by_role("button", name=re.compile(r"^(confirm|roll ?back)", re.I))
    if confirm.count():
        confirm.last.click()
    page.wait_for_timeout(6000)


def main() -> None:
    os.makedirs(OUT_DIR, exist_ok=True)
    with sync_playwright() as p:
        browser = p.chromium.launch()
        scene(browser, "deploy-live", deploy_live)
        scene(browser, "rollback", rollback)
        browser.close()


if __name__ == "__main__":
    main()
