#!/usr/bin/env python3
"""Shrink PNGs written since a start time, in parallel. Usage: optimize.py OUT_DIR START_EPOCH"""
import io
import os
import sys
from multiprocessing import Pool

from PIL import Image

LIMIT = 400 * 1024


def shrink(path):
    img = Image.open(path).convert("RGB")
    buf = io.BytesIO()
    img.save(buf, format="PNG", optimize=True)
    if buf.tell() > LIMIT:
        img = img.quantize(colors=256, method=Image.Quantize.FASTOCTREE).convert("RGB")
        buf = io.BytesIO()
        img.save(buf, format="PNG", optimize=True)
    with open(path, "wb") as f:
        f.write(buf.getvalue())
    return f"wrote {os.path.basename(path)} ({len(buf.getvalue()) // 1024} KB)"


if __name__ == "__main__":
    out_dir, started = sys.argv[1], float(sys.argv[2])
    paths = [
        os.path.join(out_dir, n)
        for n in sorted(os.listdir(out_dir))
        if n.endswith(".png") and os.path.getmtime(os.path.join(out_dir, n)) >= started
    ]
    with Pool(min(len(paths), os.cpu_count() or 2) or 1) as pool:
        for line in pool.imap(shrink, paths):
            print(line)
