#!/usr/bin/env python3
"""Split the expanded shot config into per-theme, per-worker chunk files.

Usage: shard.py CFG OUTPREFIX WORKERS ONLY FLOOR_MS READY_JS THEMES
Writes OUTPREFIX.<theme>.<n>.yml and prints each path on its own line.
"""
import os
import re
import sys

cfg, prefix, workers, only, floor_ms, wait_for, themes = sys.argv[1:8]
workers = max(1, int(workers))
wanted = {s.strip() for s in only.split(",") if s.strip()}

text = open(cfg).read()
blocks = re.split(r"(?m)^(?=- output:)", text)
blocks = [b.rstrip() + "\n" for b in blocks if b.startswith("- output:")]


def stem(block):
    m = re.search(r"(?m)^- output:\s*(\S+)", block)
    return os.path.basename(m.group(1))[: -len(".png")] if m else ""


if wanted:
    blocks = [b for b in blocks if stem(b) in wanted]
if not blocks:
    sys.exit("no shots match ONLY")


def tune(block):
    # Old fixed wait becomes a floor; the page-ready condition does the real waiting.
    block = re.sub(r"(?m)^  wait: \d+$", f"  wait: {floor_ms}", block)
    if "  wait:" not in block:
        block += f"  wait: {floor_ms}\n"
    # wait_for would need eval, which the app CSP blocks; poll inside the javascript step instead.
    poll = (
        "await new Promise(r => { const t0 = Date.now(); const tick = () => { "
        f"if (({wait_for}) || Date.now() - t0 > 20000) r(); else setTimeout(tick, 100); }}; tick(); }})"
    )
    return re.sub(
        r'(?m)^(  javascript: ")(.*)("\s*)$',
        lambda m: f"{m.group(1)}(async () => {{ {m.group(2)}; {poll}; }})(){m.group(3)}",
        block,
    )


for theme in [t.strip() for t in themes.split(",") if t.strip()]:
    chunks = [[] for _ in range(workers)]
    for i, b in enumerate(blocks):
        b = tune(b)
        if theme == "light":
            b = re.sub(r"(?m)^(- output:.*)\.png$", r"\1-light.png", b)
        chunks[i % workers].append(b)
    for n, chunk in enumerate(chunks):
        if chunk:
            path = f"{prefix}.{theme}.{n}.yml"
            open(path, "w").write("\n".join(chunk))
            print(path)
