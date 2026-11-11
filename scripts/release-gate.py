"""Bind a dispatch to a complete CI run and, for promotion, its rehearsal artifacts."""
import json
import os
from pathlib import Path
import subprocess
import urllib.parse


def gh(*args):
    return subprocess.check_output(["gh", *args], text=True).strip()


def api(path):
    return json.loads(gh("api", path))


repo = os.environ["GITHUB_REPOSITORY"]
base = f"repos/{repo}"
mode = os.environ["MODE"]
candidate = Path(os.environ["CANDIDATE"])
assert os.environ["GITHUB_REF"] == "refs/heads/main", "dispatch from main only"
if mode == "rehearse":
    source = api(f"{base}/commits/{urllib.parse.quote(os.environ['SOURCE_REF'], safe='')}")["sha"]
    assert source == os.environ["GITHUB_SHA"], "rehearse the exact main dispatch revision"
else:
    run_id = os.environ["CANDIDATE_RUN"]
    assert run_id.isdigit() and int(run_id) > 0
    run = api(f"{base}/actions/runs/{run_id}")
    workflow = api(f"{base}/actions/workflows/release.yml")
    assert run["workflow_id"] == workflow["id"]
    assert run["event"] == "workflow_dispatch" and run["head_branch"] == "main"
    assert run["conclusion"] == "success", "rehearsal must have completed successfully"
    gh("run", "download", run_id, "--repo", repo, "--name", "candidate-0.11.0", "--dir", str(candidate))
    receipt = json.loads((candidate / "candidate.json").read_text())
    assert receipt["mode"] == "rehearse" and receipt["version"] == "0.11.0"
    assert receipt["run_id"] == run_id and receipt["source"] == run["head_sha"]
    source = receipt["source"]
    if mode == "provenance":
        assert source == os.environ["GITHUB_SHA"], "provenance must identify the actual dispatch revision"
    assert api(base)["private"] is False and repo.lower() == "bitspark/horos"
    for tag in ("v0.11.0", "core/go/v0.11.0"):
        assert api(f"{base}/commits/{urllib.parse.quote(tag, safe='')}")["sha"] == source, tag
    # Tagged release source must be reachable from main, even for a later retry.
    comparison = api(f"{base}/compare/{source}...main")
    assert comparison["status"] in ("ahead", "identical")

required = {f"check ({target})" for target in ("public", "go", "rust", "typescript", "python", "conformance")}
runs = api(f"{base}/actions/workflows/ci.yml/runs?head_sha={source}&per_page=100")["workflow_runs"]
for run in runs:
    if run["head_sha"] != source or run["head_branch"] != "main" or run["event"] not in ("push", "workflow_dispatch") or run["conclusion"] != "success":
        continue
    jobs = api(f"{base}/actions/runs/{run['id']}/jobs?per_page=100")["jobs"]
    if required <= {job["name"] for job in jobs if job["conclusion"] == "success"}:
        ci = run["html_url"]
        break
else:
    raise AssertionError(f"no complete passing CI run for {source}")

with open(os.environ["GITHUB_OUTPUT"], "a") as out:
    out.write(f"source={source}\nci={ci}\n")
print(json.dumps({"source": source, "ci": ci, "mode": mode}))
