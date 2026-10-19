#!/usr/bin/env python3
"""Measure dashboard load cost against a seeded control plane (see perf-measure.sh).

Usage: perf-measure.py <base_url> <storage_state.json> [runs]

Per page it reports the median of N cold loads: time to interactive (first
moment after load where the main thread is idle for 500ms, approximated by
the last long task end or network idle, whichever is later), long task
count and total blocking time, JS heap after load, and API request count.
Chromium only, headless, so numbers are comparable between runs on one
machine, not across machines.
"""
import json
import os
import re
import statistics
import sys
import time

from playwright.sync_api import sync_playwright

BASE = sys.argv[1]
STATE = sys.argv[2]
RUNS = int(sys.argv[3]) if len(sys.argv) > 3 else 3
VERBOSE = bool(os.environ.get("PERF_VERBOSE"))

OBSERVER = """
window.__longTasks = [];
new PerformanceObserver((l) => {
  for (const e of l.getEntries()) window.__longTasks.push([e.startTime, e.duration]);
}).observe({type: 'longtask', buffered: true});
"""

PAGES = [
    ("dashboard home", "/"),
    ("apps list", "/apps"),
    ("deploys list", "/deployments"),
    ("logs page", "/apps/perf-app-0001/logs"),
    ("databases list", "/databases"),
    ("nodes list", "/nodes"),
]
HEAVY = re.compile(r"(AreaChart|LineChart|BarChart|MetricChart|xterm|addon-|cytoscape|elk-|Terminal|LogViewer)")
THROTTLE = float(os.environ.get("PERF_CPU_THROTTLE", "4"))


def throttle(ctx, page):
    if THROTTLE > 1:
        ctx.new_cdp_session(page).send("Emulation.setCPUThrottlingRate", {"rate": THROTTLE})


def load(browser, path):
    ctx = browser.new_context(storage_state=STATE, viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    throttle(ctx, page)
    page.add_init_script(OBSERVER)
    reqs = {"n": 0, "bytes": 0}

    def on_resp(r):
        if "/api/" in r.url:
            reqs["n"] += 1
            try:
                size = len(r.body())
            except Exception:
                size = 0
            reqs["bytes"] += size
            if VERBOSE:
                print(f"  {r.status} {size:>8}B {r.url.replace(BASE, '')}", file=sys.stderr)

    page.on("response", on_resp)
    t0 = time.time()
    page.goto(BASE + path, wait_until="commit")
    try:
        page.wait_for_load_state("networkidle", timeout=60000)
    except Exception:
        pass
    idle = (time.time() - t0) * 1000
    if VERBOSE:
        print(f"{path} -> {page.url}", file=sys.stderr)
    tasks = page.evaluate("window.__longTasks")
    blocking = sum(max(0, d - 50) for _, d in tasks)
    last_task_end = max([s + d for s, d in tasks], default=0)
    heap = page.evaluate("performance.memory ? performance.memory.usedJSHeapSize : 0")
    js = page.evaluate(
        "performance.getEntriesByType('resource').filter(e => e.name.endsWith('.js')).map(e => [e.name, e.decodedBodySize])"
    )
    heavy = sorted({n.rsplit("/", 1)[-1] for n, _ in js if HEAVY.search(n)})
    if VERBOSE:
        print(f"  heavy chunks: {heavy}", file=sys.stderr)
        top = sorted(js, key=lambda e: -e[1])[:5]
        print("  biggest js:", [(n.rsplit("/", 1)[-1], int(b / 1024)) for n, b in top], file=sys.stderr)
    ctx.close()
    return {
        "js_kb": sum(b for _, b in js) / 1024,
        "heavy_chunks": len(heavy),
        "tti_ms": max(idle, last_task_end),
        "long_tasks": len(tasks),
        "tbt_ms": blocking,
        "heap_mb": heap / 1048576,
        "api_requests": reqs["n"],
        "api_kb": reqs["bytes"] / 1024,
    }


def palette(browser):
    ctx = browser.new_context(storage_state=STATE, viewport={"width": 1440, "height": 900})
    page = ctx.new_page()
    throttle(ctx, page)
    page.add_init_script(OBSERVER)
    reqs = {"n": 0}
    page.on("response", lambda r: reqs.__setitem__("n", reqs["n"] + ("/api/" in r.url)))
    page.goto(BASE + "/", wait_until="networkidle")
    page.wait_for_timeout(1500)
    base_reqs = reqs["n"]
    page.evaluate("window.__longTasks.length = 0")
    t0 = time.time()
    page.keyboard.press("Control+k")
    box = page.get_by_role("dialog").get_by_role("combobox").first
    box.wait_for(timeout=10000)
    box.press_sequentially("perf-app-09", delay=40)
    page.get_by_role("dialog").get_by_text("perf-app-09", exact=False).nth(1).wait_for(timeout=15000)
    typed = (time.time() - t0) * 1000 - 40 * 11
    page.wait_for_timeout(500)
    if VERBOSE:
        print("palette input value:", box.input_value(), "reqs", reqs["n"] - base_reqs, file=sys.stderr)
    elapsed = typed
    tasks = page.evaluate("window.__longTasks")
    out = {
        "tti_ms": elapsed,
        "long_tasks": len(tasks),
        "tbt_ms": sum(max(0, d - 50) for _, d in tasks),
        "heap_mb": page.evaluate("performance.memory ? performance.memory.usedJSHeapSize : 0") / 1048576,
        "api_requests": reqs["n"] - base_reqs,
        "api_kb": 0,
        "js_kb": 0,
        "heavy_chunks": 0,
    }
    ctx.close()
    return out


def med(rows, key):
    return statistics.median(r[key] for r in rows)


def main():
    results = {}
    with sync_playwright() as p:
        browser = p.chromium.launch(args=["--enable-precise-memory-info"])
        for name, path in PAGES:
            if os.environ.get("PERF_ONLY") == "palette":
                break
            load(browser, path)
            rows = [load(browser, path) for _ in range(RUNS)]
            results[name] = {k: med(rows, k) for k in rows[0]}
        rows = [palette(browser) for _ in range(RUNS)]
        results["search palette"] = {k: med(rows, k) for k in rows[0]}
        browser.close()
    print(json.dumps(results, indent=2))
    print()
    print("| page | TTI ms | long tasks | TBT ms | JS heap MB | API requests | API KB | JS KB | heavy chunks |")
    print("|---|---|---|---|---|---|---|---|---|")
    for name, r in results.items():
        print(f"| {name} | {r['tti_ms']:.0f} | {r['long_tasks']:.0f} | {r['tbt_ms']:.0f} | {r['heap_mb']:.0f} | {r['api_requests']:.0f} | {r['api_kb']:.0f} | {r['js_kb']:.0f} | {r['heavy_chunks']:.0f} |")


if __name__ == "__main__":
    main()
