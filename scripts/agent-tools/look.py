#!/usr/bin/env python3
"""Several small code lookups in one call, with capped output.

Every tool call of a long-lived agent re-sends its whole context, so ten
one-line lookups cost ten times what one batched call costs. Give all the
lookups you need now as arguments:

  FILE:A-B                   lines A..B of FILE (numbered)
  go:PATH:Name[,Name...]     Go declarations by name in a file or package dir
                             (func, Type.Method, type, var, const; doc comment included)
  gotest:PATH:Name[,...]     the same, searching _test.go files too
  rg:PATH:PATTERN            rg -n matches of PATTERN under PATH (at most --rg-max lines)
  rgl:PATH:PATTERN           files under PATH matching PATTERN, with match counts
  rgg:GLOB:PATH:PATTERN      rg -n restricted to GLOB

Relative paths resolve against --root (default: the current directory).
Prints "### STOP" first when the run that owns the worktree is stopping.
"""

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import wave_mail  # noqa: E402

TOOLS = Path(os.environ.get("WAVES_ROOT", wave_mail.DEFAULT_ROOT)).parent / ".tools"


def godecl_bin():
    """Build godecl into the shared tools dir when missing or older than its source."""
    src = HERE / "godecl" / "main.go"
    out = TOOLS / "godecl"
    if not out.exists() or out.stat().st_mtime < src.stat().st_mtime:
        TOOLS.mkdir(parents=True, exist_ok=True)
        tmp = out.with_name(f".godecl.{os.getpid()}")
        subprocess.run(["go", "build", "-o", str(tmp), "."], cwd=src.parent, check=True,
                       stdout=subprocess.DEVNULL, env=dict(os.environ, GOWORK="off"))
        os.replace(tmp, out)
    return str(out)


def cap(lines, n, hint):
    if len(lines) <= n:
        return lines
    return lines[:n] + [f"... ({len(lines) - n} more lines cut; {hint})"]


def do_range(path, a, b, maxl):
    try:
        text = Path(path).read_text(encoding="utf-8", errors="replace").split("\n")
    except OSError as e:
        return [f"error: {e}"]
    a = max(1, a)
    b = min(len(text), b)
    rows = [f"{i}\t{text[i - 1]}" for i in range(a, b + 1)]
    return cap(rows, maxl, f"read a narrower range than {a}-{b}")


def do_go(path, names, tests, maxl):
    try:
        binary = godecl_bin()
    except (OSError, subprocess.CalledProcessError) as e:
        # One failed build costs this spec only, never the rest of the batch.
        return [f"error: godecl build failed ({e}); use rg or gopls for this lookup"]
    args = [binary, "-max", str(maxl)] + (["-tests"] if tests else []) + [path] + names
    r = subprocess.run(args, capture_output=True, text=True)
    return (r.stdout + r.stderr).rstrip("\n").split("\n")


def do_rg(path, pattern, glob, files_only, maxl):
    args = ["rg", "--no-heading", "--color", "never"]
    args += ["-c"] if files_only else ["-n", "-m", "20"]
    if glob:
        args += ["-g", glob]
    args += ["-e", pattern, path]
    r = subprocess.run(args, capture_output=True, text=True)
    lines = [l[:300] for l in r.stdout.rstrip("\n").split("\n") if l]
    if not lines:
        return [r.stderr.strip() or "(no matches)"]
    return cap(lines, maxl, "narrow PATH or PATTERN, or use rgl: first")


def resolve(root, p):
    return p if os.path.isabs(p) else str(Path(root) / p)


def run_spec(spec, root, a):
    m = re.fullmatch(r"(.+):(\d+)-(\d+)", spec)
    kind, _, rest = spec.partition(":")
    if kind in ("go", "gotest") and rest:
        path, _, names = rest.rpartition(":")
        names = [n for n in names.split(",") if n]
        path = resolve(root, path)
        return do_go(path, names, kind == "gotest", a.max)
    if kind in ("rg", "rgl") and rest:
        path, _, pattern = rest.partition(":")
        return do_rg(resolve(root, path), pattern, None, kind == "rgl", a.rg_max)
    if kind == "rgg" and rest:
        glob, _, rest2 = rest.partition(":")
        path, _, pattern = rest2.partition(":")
        return do_rg(resolve(root, path), pattern, glob, False, a.rg_max)
    if m:
        return do_range(resolve(root, m.group(1)), int(m.group(2)), int(m.group(3)), a.max)
    return [f"error: unknown spec {spec!r} (see --help)"]


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("specs", nargs="+")
    p.add_argument("--root", default=os.getcwd())
    p.add_argument("--max", type=int, default=150, help="line cap per range or declaration")
    p.add_argument("--rg-max", type=int, default=40, help="line cap per rg spec")
    p.add_argument("--total", type=int, default=600, help="line cap for the whole output")
    a = p.parse_args(argv)
    lines = []
    stop = wave_mail.stop_banner([a.root] + re.findall(r"(/[^:\s]+)", " ".join(a.specs)))
    if stop:
        lines.append(stop)
    for spec in a.specs:
        lines.append(f"### {spec}")
        lines += run_spec(spec, a.root, a)
    print("\n".join(cap(lines, a.total, "split the lookups over two calls")))


if __name__ == "__main__":
    main()
