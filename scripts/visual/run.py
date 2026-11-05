#!/usr/bin/env python3
"""Screenshots plus geometry assertions for the dashboard at three widths.

Needs a running dev-mode control plane and web server: use scripts/visual/run.sh,
which starts both on free ports with a throwaway data dir. Exits 1 when any
geometry assertion fails and prints a markdown summary.

Env: LEVELRAIL_VISUAL_URL (web), LEVELRAIL_VISUAL_API (api), LEVELRAIL_VISUAL_USER,
LEVELRAIL_VISUAL_PASS, LEVELRAIL_VISUAL_TOKEN (write token seeded by dev fixtures).
Args: --out DIR, --only ROUTE_ID[,ID], --widths 1280,390
"""
import argparse
import json
import os
import sys
import urllib.request
from pathlib import Path

from playwright.sync_api import sync_playwright

HERE = Path(__file__).resolve().parent
WEB = os.environ["LEVELRAIL_VISUAL_URL"].rstrip("/")
API = os.environ["LEVELRAIL_VISUAL_API"].rstrip("/")
USER = os.environ.get("LEVELRAIL_VISUAL_USER", "dev")
PASSWORD = os.environ.get("LEVELRAIL_VISUAL_PASS", "dev")
TOKEN = os.environ.get("LEVELRAIL_VISUAL_TOKEN", "dev-root-token")
SETTLE_MS = int(os.environ.get("LEVELRAIL_VISUAL_SETTLE_MS", "700"))

FREEZE_CSS = (
    "*,*::before,*::after{animation:none!important;transition:none!important;"
    "caret-color:transparent!important}"
)

# Runs in the page. Returns {fail: [...], warn: [...]} of one-line strings.
GEOMETRY_JS = r"""
() => {
  const fail = [], warn = [];
  const vw = document.documentElement.clientWidth;
  const label = (el) => {
    const id = el.id ? '#' + el.id : '';
    const cls = (typeof el.className === 'string' ? el.className : '').trim().split(/\s+/).slice(0, 2).join('.');
    return el.tagName.toLowerCase() + id + (cls ? '.' + cls : '');
  };
  const srOnly = (el) => el.closest('.sr-only, [class*="sr-only"]') !== null;
  const visible = (el, cs) => cs.display !== 'none' && cs.visibility !== 'hidden' && cs.opacity !== '0';
  const rectOf = (el) => el.getBoundingClientRect();
  const clipsX = (el) => {
    for (let p = el.parentElement; p; p = p.parentElement) {
      const o = getComputedStyle(p).overflowX;
      if (o === 'auto' || o === 'scroll' || o === 'hidden' || o === 'clip') return true;
    }
    return false;
  };

  const sw = document.documentElement.scrollWidth;
  if (sw > vw + 1) fail.push(`horizontal overflow: scrollWidth ${sw} > viewport ${vw}`);

  const all = Array.from(document.body.querySelectorAll('*'));
  const wide = [];
  for (const el of all) {
    const cs = getComputedStyle(el);
    if (!visible(el, cs) || cs.position === 'fixed') continue;
    const r = rectOf(el);
    if (r.width === 0 || r.height === 0) continue;
    if (r.right > vw + 1 && !clipsX(el)) wide.push([el, r]);
  }
  // Report only outermost offenders so one wide row is not 40 lines.
  for (const [el, r] of wide) {
    if (wide.some(([o]) => o !== el && o.contains(el))) continue;
    fail.push(`outside viewport: ${label(el)} right ${Math.round(r.right)} > ${vw}`);
  }

  // Children that stick out of a parent that does not clip them.
  for (const el of all) {
    const cs = getComputedStyle(el);
    if (!visible(el, cs) || cs.position === 'absolute' || cs.position === 'fixed') continue;
    const p = el.parentElement;
    if (!p || p === document.body) continue;
    const pcs = getComputedStyle(p);
    if (pcs.display === 'contents' || pcs.overflowX !== 'visible' || pcs.position === 'static' && pcs.display === 'inline') continue;
    const r = rectOf(el), pr = rectOf(p);
    if (r.width === 0 || pr.width === 0 || r.right > vw + 1) continue;
    if (r.left < pr.left - 2 || r.right > pr.right + 2) {
      if (el.tagName !== 'svg' && !srOnly(el) && (el.tagName === 'IMG' || el.querySelector('input,button,a,img')))
        warn.push(`leaves container: ${label(el)} [${Math.round(r.left)},${Math.round(r.right)}] in ${label(p)} [${Math.round(pr.left)},${Math.round(pr.right)}]`);
    }
  }

  // Absolute nodes with no insets sit at their static position, i.e. stacked in flow.
  const abs = all.filter((el) => {
    const cs = getComputedStyle(el);
    return cs.position === 'absolute' && visible(el, cs) &&
      cs.top === 'auto' && cs.left === 'auto' && cs.right === 'auto' && cs.bottom === 'auto' &&
      cs.transform === 'none' && rectOf(el).width > 0;
  });
  for (const el of abs) {
    const r = rectOf(el);
    const flowSibling = Array.from(el.parentElement ? el.parentElement.children : []).find((s) => {
      if (s === el) return false;
      const cs = getComputedStyle(s);
      if (cs.position === 'absolute' || cs.position === 'fixed' || !visible(s, cs)) return false;
      const sr = rectOf(s);
      const ix = Math.min(r.right, sr.right) - Math.max(r.left, sr.left);
      const iy = Math.min(r.bottom, sr.bottom) - Math.max(r.top, sr.top);
      return sr.width > 0 && sr.height > 0 && ix > 2 && iy > 2;
    });
    if (flowSibling) fail.push(`absolute node stacked in flow: ${label(el)} overlaps ${label(flowSibling)}`);
  }
  const seen = new Map();
  for (const el of abs) {
    const r = rectOf(el);
    const key = (el.parentElement ? all.indexOf(el.parentElement) : -1) + ':' + Math.round(r.left) + ':' + Math.round(r.top);
    if (seen.has(key)) fail.push(`absolute nodes share one spot: ${label(seen.get(key))} and ${label(el)}`);
    else seen.set(key, el);
  }

  // Clipped text and broken images are warnings, they need a human look.
  for (const el of all) {
    const cs = getComputedStyle(el);
    if (!visible(el, cs) || srOnly(el)) continue;
    if (el.tagName === 'IMG' && el.complete && el.naturalWidth === 0) warn.push(`broken image: ${label(el)}`);
    if (el.children.length === 0 && el.textContent.trim().length > 1 && el.scrollWidth > el.clientWidth + 1 &&
        (cs.overflowX === 'hidden' || cs.overflowX === 'clip') && cs.textOverflow !== 'ellipsis' &&
        cs.display !== 'inline')
      warn.push(`clipped text: ${label(el)} "${el.textContent.trim().slice(0, 30)}"`);
  }
  return { fail: fail.slice(0, 12), warn: warn.slice(0, 12) };
}
"""


def seed_apps(apps):
    for app in apps:
        req = urllib.request.Request(
            f"{API}/api/v1/apps",
            data=json.dumps(app).encode(),
            method="POST",
            headers={"Authorization": f"Bearer {TOKEN}", "Content-Type": "application/json"},
        )
        try:
            urllib.request.urlopen(req, timeout=20).read()
        except Exception as exc:  # noqa: BLE001
            print(f"visual: seeding {app['name']} failed: {exc}", file=sys.stderr)


def login(browser, state_path):
    ctx = browser.new_context()
    page = ctx.new_page()
    page.goto(f"{WEB}/login")
    page.fill("#login-username", USER)
    page.get_by_role("button", name="Continue").click()
    page.wait_for_selector("#login-password", timeout=20000)
    page.fill("#login-password", PASSWORD)
    page.click("button[type=submit]")
    page.wait_for_url(lambda url: "/login" not in url, timeout=20000)
    ctx.storage_state(path=str(state_path))
    ctx.close()


def prepare(page, route, width):
    """Apply the route's action; returns extra fail strings."""
    extra = []
    action = route.get("action")
    if action == "palette":
        page.locator('[aria-keyshortcuts="Control+K Meta+K"]').first.click()
        try:
            page.get_by_role("dialog").first.wait_for(timeout=5000)
        except Exception:  # noqa: BLE001
            extra.append("command palette did not open")
    elif action == "sidebar":
        if not page.get_by_text("Build and run", exact=True).first.is_visible():
            page.get_by_label("Toggle sidebar").first.click()
            page.wait_for_timeout(400)
        names = ("Build and run", "Data", "Network", "Fleet and cost", "Security", "AI")
        missing = [n for n in names if page.get_by_text(n, exact=True).count() == 0]
        if missing:
            extra.append("sidebar groups missing: " + ", ".join(missing))
        for name in names:
            try:
                page.get_by_text(name, exact=True).first.click(timeout=1500)
                page.wait_for_timeout(120)
            except Exception:  # noqa: BLE001
                pass
    if route.get("advisory"):
        sect = page.locator('section[aria-label="Advisory"]')
        if sect.count() == 0:
            extra.append("status advisory block not rendered")
        else:
            box = sect.first.bounding_box()
            if box and (box["width"] < 100 or box["height"] < 16):
                extra.append(f"status advisory block collapsed ({box['width']:.0f}x{box['height']:.0f})")
    return extra


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True)
    ap.add_argument("--only", default="")
    ap.add_argument("--widths", default="")
    args = ap.parse_args()

    cfg = json.loads((HERE / "routes.json").read_text())
    widths = [int(w) for w in args.widths.split(",")] if args.widths else cfg["widths"]
    only = set(filter(None, args.only.split(",")))
    routes = [r for r in cfg["routes"] if not only or r["id"] in only]
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)
    for old in out.glob("*.png"):
        old.unlink()

    seed_apps(cfg["fixtureApps"])
    rows = []
    with sync_playwright() as p:
        try:
            browser = p.chromium.launch(channel="chrome")
        except Exception:  # noqa: BLE001
            browser = p.chromium.launch()
        state = out / ".state.json"
        login(browser, state)
        for route in routes:
            for width in widths:
                ctx = browser.new_context(
                    viewport={"width": width, "height": 900},
                    reduced_motion="reduce",
                    color_scheme="dark",
                    storage_state=None if route.get("anonymous") else str(state),
                )
                page = ctx.new_page()
                console = []
                page.on("pageerror", lambda e, c=console: c.append(f"page error: {str(e)[:100]}"))
                page.on("console", lambda m, c=console: m.type == "error" and "Failed to load resource" not in m.text and c.append(f"console error: {m.text[:100]}"))
                page.on("response", lambda r, c=console: r.status >= 400 and "/@fs/" not in r.url and c.append(f"http {r.status}: {r.url.split('//', 1)[-1].split('/', 1)[-1][:60]}"))
                fails, warns = [], []
                try:
                    page.goto(f"{WEB}{route['path']}", wait_until="load", timeout=45000)
                    try:
                        page.wait_for_load_state("networkidle", timeout=8000)
                    except Exception:  # noqa: BLE001
                        warns.append("network never idle in 8s")
                    page.add_style_tag(content=FREEZE_CSS)
                    fails += prepare(page, route, width)
                    page.wait_for_timeout(SETTLE_MS)
                    geo = page.evaluate(GEOMETRY_JS)
                    fails += geo["fail"]
                    warns += geo["warn"]
                    if "/login" in page.url and not route.get("anonymous"):
                        fails.append("redirected to login")
                    page.screenshot(path=str(out / f"{route['id']}-{width}.png"), full_page=True)
                except Exception as exc:  # noqa: BLE001
                    fails.append(f"run error: {str(exc)[:120]}")
                warns += console[:3]
                rows.append((route["id"], width, fails, warns))
                ctx.close()
        browser.close()
        state.unlink(missing_ok=True)

    failed = sum(1 for r in rows if r[2])
    lines = [f"## Visual run: {len(rows)} captures, {failed} failing", "", "| route | width | result | details |", "|---|---|---|---|"]
    for rid, w, fails, warns in rows:
        res = "FAIL" if fails else ("warn" if warns else "ok")
        det = "<br>".join((fails + warns)[:5]).replace("|", "/")
        lines.append(f"| {rid} | {w} | {res} | {det} |")
    lines += ["", f"Screenshots: {out.resolve()}"]
    report = "\n".join(lines)
    (out / "report.md").write_text(report + "\n")
    print(report)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
