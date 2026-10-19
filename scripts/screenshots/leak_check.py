#!/usr/bin/env python3
"""Scan every page a shots config will capture for real-looking identifiers.

Usage: leak_check.py <rendered_shots.yml> <storage_state.json>

Loads each `url:` in the config through the saved login session and fails
if the rendered text contains a public IPv4 address (dotted or dashed, as in
sslip.io hostnames), an email address, a sslip.io host, or this machine's
hostname. Private and loopback addresses (Docker bridges, mesh) are allowed. Demo data only: a hit means a real value leaked into a shot, so
the affected page needs a demo-safe value before the screenshot is kept.
"""
import ipaddress
import re
import socket
import sys

from playwright.sync_api import sync_playwright

IPV4 = re.compile(r"\b\d{1,3}(?:[.-]\d{1,3}){3}\b")
EMAIL = re.compile(r"[\w.+-]+@[\w-]+\.[\w.]+")
SSLIP_HOST = re.compile(r"\b((?:\d{1,3}-){3}\d{1,3})\.sslip\.io", re.IGNORECASE)
# RFC 2606 reserved domains are demo values.
RESERVED_DOMAINS = ("example.com", "example.org", "example.net", ".test", ".invalid")
# Text the app shows instead of the page: a shot of these is useless, so they fail the run too.
# Translation keys that were never resolved, e.g. steps.sign_in_recovery.title.
RAW_KEY = re.compile(r"[a-z_]+(?:\.[a-z_]+)+")
UPDATE_BANNER = re.compile(r"Version v?\d[\w.\-]* is available", re.IGNORECASE)
ERROR_MARKERS = ("something went wrong", "rate limit exceeded", "page not found")


def is_demo(ip):
    """Private, loopback, and documentation addresses (Docker bridges, mesh, RFC 5737) reveal no host."""
    try:
        addr = ipaddress.ip_address(ip.replace("-", "."))
    except ValueError:
        return False
    return addr.is_private or addr.is_loopback or addr.is_unspecified


def main() -> int:
    config_path, state_path = sys.argv[1], sys.argv[2]
    urls = []
    with open(config_path) as f:
        for line in f:
            stripped = line.strip()
            if stripped.startswith("url:"):
                urls.append(stripped[len("url:"):].strip())

    local = socket.gethostname()
    host_names = {local.lower(), local.split(".")[0].lower()}
    host_names = {name for name in host_names if len(name) > 3}
    failures = []
    with sync_playwright() as p:
        browser = p.chromium.launch()
        context = browser.new_context(storage_state=state_path)
        page = context.new_page()
        for url in urls:
            try:
                page.goto(url, wait_until="domcontentloaded", timeout=60000)
                page.wait_for_load_state("networkidle", timeout=20000)
            except Exception:  # noqa: BLE001 - a page that never settles is still checked
                pass
            page.wait_for_timeout(1500)
            text = page.inner_text("body")
            hits = {ip for ip in IPV4.findall(text) if not is_demo(ip)}
            hits |= {m for m in EMAIL.findall(text) if not m.lower().endswith(RESERVED_DOMAINS)}
            hits |= {host + ".sslip.io" for host in SSLIP_HOST.findall(text) if not is_demo(host)}
            lowered = text.lower()
            hits |= {name for name in host_names if name in lowered}
            hits |= {f"error page: {marker}" for marker in ERROR_MARKERS if marker in lowered}
            hits |= {f"raw i18n key: {line.strip()}" for line in text.splitlines() if RAW_KEY.fullmatch(line.strip())}
            if re.search(r"\bsteps\.[a-z_]+", text):
                hits.add("literal steps.<key> text")
            if UPDATE_BANNER.search(text):
                hits.add("version update banner")
            if hits:
                failures.append((url, sorted(hits)))
        browser.close()

    for url, hits in failures:
        print(f"LEAK {url}: {', '.join(hits)}")
    print(f"leak check: {len(urls)} pages, {len(failures)} with identifiers")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
