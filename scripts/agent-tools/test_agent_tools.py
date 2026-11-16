"""Tests for the token-saving lane tools: wave_mail worktree/pr-state/ci-wait/stop
signalling and look.py. GitHub is replaced by a fake gh script."""
import json
import os
import shutil
import subprocess
import sys
import tempfile
import textwrap
import time
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
MB = HERE / "wave_mail.py"
LOOK = HERE / "look.py"

FAKE_GH = textwrap.dedent('''\
    #!/usr/bin/env python3
    import json, os, sys
    data = json.load(open(os.environ["FAKE_GH_DATA"]))
    key = " ".join(sys.argv[1:])
    for k, v in data.items():
        if key.startswith(k):
            print(json.dumps(v)); sys.exit(0)
    sys.stderr.write("fake gh: no answer for " + key); sys.exit(1)
''')


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())
        self.waves = self.tmp / "wt" / ".waves"
        self.waves.mkdir(parents=True)
        gh = self.tmp / "gh"
        gh.write_text(FAKE_GH)
        gh.chmod(0o755)
        self.gh_data = self.tmp / "gh.json"
        self.env = dict(os.environ, WAVES_ROOT=str(self.waves), WAVES_GH=str(gh), FAKE_GH_DATA=str(self.gh_data),
                        WAVES_POLL="0.05", WAVES_CI_POLL="0.05")

    def tearDown(self):
        shutil.rmtree(self.tmp)

    def gh(self, data):
        self.gh_data.write_text(json.dumps(data))

    def run_py(self, script, *args, check=True):
        r = subprocess.run([sys.executable, str(script), *map(str, args)], capture_output=True, text=True, env=self.env)
        if check and r.returncode:
            self.fail(f"{script.name} {args}: rc={r.returncode} {r.stderr}")
        return r

    def mb(self, *args, check=True):
        r = self.run_py(MB, *args, check=check)
        return json.loads(r.stdout) if r.stdout.strip() else None


class WaveMailTools(Base):
    def test_set_takes_several_pairs(self):
        st = self.mb("state", "--run", "r", "--lane", "a", "--set", "round=4", "stage=merge")
        self.assertEqual((st["round"], st["stage"]), (4, "merge"))

    def test_every_output_of_a_stopping_run_says_stop(self):
        self.mb("state", "--run", "r", "--lane", "a", "--set", "stage=impl")
        self.assertNotIn("stop", self.mb("state", "--run", "r", "--lane", "a"))
        self.mb("stop", "--run", "r")
        self.assertTrue(self.mb("state", "--run", "r", "--lane", "a")["stop"])

    def test_stop_banner_names_lane_worktrees_only(self):
        wt = self.tmp / "wt" / "wf-a"
        (wt / "x").mkdir(parents=True)
        self.mb("state", "--run", "r", "--lane", "a", "--set", f"worktree={wt}")
        self.assertEqual(self.run_py(MB, "stopcheck", wt / "x").returncode, 0)
        self.mb("stop", "--run", "r")
        r = self.run_py(MB, "stopcheck", wt / "x", check=False)
        self.assertEqual(r.returncode, 3)
        self.assertIn("### STOP", r.stdout)
        self.assertEqual(self.run_py(MB, "stopcheck", self.tmp, check=False).returncode, 0)
        review = self.tmp / "wt" / "review-a"
        review.mkdir()
        self.assertEqual(self.run_py(MB, "stopcheck", review, check=False).returncode, 3)

    def test_stop_banner_ignores_an_older_run_on_the_same_worktree(self):
        wt = self.tmp / "wt" / "wf-a"
        wt.mkdir(parents=True)
        self.mb("state", "--run", "old", "--lane", "a", "--set", f"worktree={wt}")
        self.mb("stop", "--run", "old")
        time.sleep(1.1)  # lane state timestamps have one-second resolution
        self.mb("state", "--run", "new", "--lane", "a", "--set", f"worktree={wt}")
        self.assertEqual(self.run_py(MB, "stopcheck", wt).returncode, 0, "the abandoned run's STOP is not the new run's")
        self.mb("stop", "--run", "new")
        r = self.run_py(MB, "stopcheck", wt, check=False)
        self.assertEqual(r.returncode, 3)
        self.assertIn("run new is stopping", r.stdout)

    def test_worktree_creates_reuses_and_removes(self):
        origin = self.tmp / "origin.git"
        go = self.tmp / "go"
        git = lambda *a, cwd=None: subprocess.run(["git", *map(str, a)], cwd=cwd, check=True, capture_output=True)
        git("init", "-q", "--bare", "-b", "main", origin)
        git("clone", "-q", origin, go)
        git("-C", go, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init")
        git("-C", go, "push", "-q", "origin", "main")
        self.env["WAVES_GO"] = str(go)
        r = self.mb("worktree", "--run", "r", "--lane", "a")
        self.assertTrue(r["created"])
        self.assertEqual(r["worktree"], str(self.tmp / "wt" / "wf-a"))
        st = self.mb("state", "--run", "r", "--lane", "a")
        self.assertEqual((st["stage"], st["branch"]), ("impl", "wf/a"))
        self.mb("state", "--run", "r", "--lane", "a", "--set", "stage=review")
        r = self.mb("worktree", "--run", "r", "--lane", "a")
        self.assertFalse(r["created"])
        self.assertEqual(self.mb("state", "--run", "r", "--lane", "a")["stage"], "review", "reuse keeps the stage")
        wt = Path(r["worktree"])
        git("-C", wt, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "work")
        git("-C", wt, "push", "-q", "origin", "wf/a")
        rv = self.mb("worktree", "--run", "r", "--lane", "a", "--review")
        self.assertEqual(rv["head"], self.mb("worktree", "--run", "r", "--lane", "a")["head"])
        self.assertEqual(rv["ahead_of_main"], 1)
        self.mb("worktree", "--run", "r", "--lane", "a", "--remove")
        self.assertFalse(wt.exists())

    def test_drain_refuses_only_new_lanes(self):
        origin = self.tmp / "origin.git"
        go = self.tmp / "go"
        git = lambda *a: subprocess.run(["git", *map(str, a)], check=True, capture_output=True)
        git("init", "-q", "--bare", "-b", "main", origin)
        git("clone", "-q", origin, go)
        git("-C", go, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "init")
        git("-C", go, "push", "-q", "origin", "main")
        self.env["WAVES_GO"] = str(go)
        self.assertTrue(self.mb("worktree", "--run", "r", "--lane", "a")["created"])
        self.mb("stop", "--run", "r", "--drain")
        self.assertFalse(self.mb("worktree", "--run", "r", "--lane", "a")["created"], "a lane under way keeps working")
        r = self.mb("worktree", "--run", "r", "--lane", "b")
        self.assertTrue(r["stop"] and r["drain"])
        self.assertFalse((self.tmp / "wt" / "wf-b").exists())
        self.assertNotIn("stop", self.mb("state", "--run", "r", "--lane", "a"), "draining is not a stop for running lanes")

    def test_pr_state_is_compact(self):
        self.gh({"api repos/fatal10110/acis_golang_bot/pulls/7": {"state": "open", "merged": False, "draft": False,
                 "head": {"sha": "h1", "ref": "wf/a"}, "base": {"ref": "main"}, "mergeable_state": "behind"},
                 "api repos/fatal10110/acis_golang_bot/compare/main...h1": {"behind_by": 3}})
        self.assertEqual(self.mb("pr-state", "--pr", "7"), {"pr": 7, "state": "open", "merged": False, "merge_sha": "", "draft": False,
                         "head": "h1", "branch": "wf/a", "mergeable_state": "behind", "behind_by": 3})

    def runs(self, *rows):
        return {"api repos/fatal10110/acis_golang_bot/commits/h1/check-runs": {"check_runs": [
            {"id": i, "name": n, "status": s, "conclusion": c, "details_url": f"https://x/actions/runs/9/job/{i}"}
            for i, (n, s, c) in enumerate(rows, 1)]}}

    def test_ci_wait_success_needs_the_required_checks(self):
        self.gh(self.runs(("CodeQL", "completed", "success")))
        r = self.mb("ci-wait", "--run", "r", "--head", "h1", "--wait", "0.2")
        self.assertEqual((r["state"], r["missing"]), ("pending", ["lint", "test"]))
        self.gh(self.runs(("go", "completed", "success"), ("lint", "completed", "success"), ("test (rest)", "completed", "success")))
        self.assertEqual(self.mb("ci-wait", "--run", "r", "--head", "h1", "--wait", "1")["state"], "success")

    def test_ci_wait_required_checks_are_configurable(self):
        self.env["WAVES_REQUIRED_CHECKS"] = "build,e2e"
        self.gh(self.runs(("build", "completed", "success")))
        r = self.mb("ci-wait", "--run", "r", "--head", "h1", "--wait", "0.2")
        self.assertEqual((r["state"], r["missing"]), ("pending", ["e2e"]))

    def test_ci_wait_reports_failures_once_all_finished(self):
        self.gh(self.runs(("go", "completed", "success"), ("lint", "completed", "success"),
                          ("test (rest)", "completed", "failure"), ("test (pool-rest)", "in_progress", None)))
        self.assertEqual(self.mb("ci-wait", "--run", "r", "--head", "h1", "--wait", "0.2")["state"], "pending")
        self.gh(self.runs(("go", "completed", "success"), ("lint", "completed", "success"),
                          ("test (rest)", "completed", "failure"), ("test (pool-rest)", "completed", "success")))
        r = self.mb("ci-wait", "--run", "r", "--head", "h1", "--wait", "1")
        self.assertEqual(r["state"], "failure")
        self.assertEqual(r["failed"], [{"name": "test (rest)", "conclusion": "failure", "run_id": "9", "job_id": "3"}])


class LookTool(Base):
    def test_batches_ranges_go_and_rg(self):
        src = self.tmp / "src"
        src.mkdir()
        (src / "a.go").write_text("package a\n\n// T is a thing.\ntype T struct{ n int }\n\n// Get returns n.\nfunc (t *T) Get() int {\n\treturn t.n\n}\n\nfunc Free() {}\n")
        if not shutil.which("go"):
            self.skipTest("no go toolchain")
        self.env["WAVES_ROOT"] = str(self.waves)
        out = self.run_py(LOOK, "--root", src, "a.go:3-4", "go:.:T.Get,T", "rg:.:Free", "rgl:.:return").stdout
        self.assertIn("4\ttype T struct{ n int }", out)
        self.assertIn("6\t// Get returns n.", out)
        self.assertIn("a.go:11:func Free() {}", out)
        self.assertRegex(out, r"a\.go:2")
        self.assertNotIn("### STOP", out)

    def test_a_failed_godecl_build_costs_only_its_spec(self):
        src = self.tmp / "src"
        src.mkdir()
        (src / "a.go").write_text("package a\n\nfunc Free() {}\n")
        bin_dir = self.tmp / "bin"  # rg but no go: the godecl build cannot run
        bin_dir.mkdir()
        (bin_dir / "rg").symlink_to(shutil.which("rg"))
        self.env["PATH"] = str(bin_dir)
        r = self.run_py(LOOK, "--root", src, "go:.:Free", "rg:.:Free", "a.go:3-3")
        self.assertIn("error: godecl build failed", r.stdout)
        self.assertIn("a.go:3:func Free() {}", r.stdout)
        self.assertIn("3\tfunc Free() {}", r.stdout)

    def test_caps_long_output(self):
        f = self.tmp / "big.txt"
        f.write_text("\n".join(f"line {i}" for i in range(1, 400)))
        out = self.run_py(LOOK, f"{f}:1-399", "--max", "20").stdout
        self.assertIn("20\tline 20", out)
        self.assertNotIn("21\tline 21", out)
        self.assertIn("more lines cut", out)


if __name__ == "__main__":
    unittest.main()
