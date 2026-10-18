"""Prompt benchmark for embodied-ai-proxy. See benchmark/README.md.

  python benchmark/bench.py run --name baseline [--prompt PROMPT.md] [--reps 5]
  python benchmark/bench.py report benchmark/results/baseline.jsonl [benchmark/results/trimmed.jsonl]
"""
import argparse
import json
import os
import shutil
import socket
import statistics
import subprocess
import sys
import time
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import tiktoken
import yaml

HERE = Path(__file__).resolve().parent
REPO = HERE.parent
BIN = REPO / "bin"
WORK = HERE / ".work"
DEFAULT_KEY_FILE = Path("~/.config/embodied-ai-proxy/openai_key").expanduser()
PRICE_IN, PRICE_OUT = 0.15, 0.60  # USD per 1M tokens, gpt-4o-mini


def load_cases(path):
    doc = yaml.safe_load(Path(path).read_text())
    return doc["defaults"], doc["cases"]


# --- scoring ---------------------------------------------------------------

def check_value(actual, rule):
    if rule == "!":
        return actual is None
    if rule == "?":
        return actual is not None
    if isinstance(rule, list):
        return actual in rule
    if actual is None:
        return False
    if isinstance(rule, str) and rule[:1] in "<>" and rule[1:2].isdigit():
        if not isinstance(actual, (int, float)):
            return False
        return actual < float(rule[1:]) if rule[0] == "<" else actual > float(rule[1:])
    if isinstance(rule, str) and rule.startswith("range:"):
        lo, hi = map(float, rule.split(":")[1:])
        return isinstance(actual, (int, float)) and lo <= actual <= hi
    return actual == rule


def strip_home(steps, expected):
    s = list(steps)
    if expected and expected[0] != "home":
        while s and s[0]["action"] == "home":
            s.pop(0)
    if expected and expected[-1] != "home":
        while s and s[-1]["action"] == "home":
            s.pop()
    return s


def score(case, result):
    """Returns (passed, reason)."""
    exp = case["expect"]
    if result.get("transport_error"):
        return False, "transport: " + result["transport_error"]
    if result.get("error"):
        return False, "pipeline: " + result["error"][:160]
    doc = result.get("parsed") or {}
    status = doc.get("status")

    if exp["status"] == "error":
        if status != "error":
            return False, "expected refusal, got recipe: " + " > ".join(s["action"] for s in doc.get("steps", []))
        allowed = exp.get("error_type")
        if allowed and doc.get("error_type") not in allowed:
            return False, f"wrong error_type {doc.get('error_type')}"
        return True, ""

    if status != "success":
        return False, f"refused ({doc.get('error_type')}): {doc.get('message', '')[:120]}"
    steps = doc.get("steps", [])
    if exp.get("no_trailing_home") and steps and steps[-1]["action"] == "home":
        return False, "ended on home while holding an object"

    seqs = exp["actions"]
    seqs = seqs if isinstance(seqs[0], list) else [seqs]
    for seq in seqs:
        s = strip_home(steps, seq)
        if [x["action"] for x in s] == seq:
            break
    else:
        return False, "actions " + " > ".join(x["action"] for x in steps)

    for i, rules in enumerate((exp.get("params") or [])[:len(s)]):
        params = s[i].get("parameters") or {}
        for key, rule in rules.items():
            if not check_value(params.get(key), rule):
                return False, f"step {i + 1} {s[i]['action']}.{key}={params.get(key)!r}, wanted {rule!r}"
    return True, ""


def signature(result):
    doc = result.get("parsed") or {}
    if doc.get("status") == "success":
        return json.dumps([[s["action"], s.get("parameters") or {}] for s in doc["steps"]], sort_keys=True)
    return json.dumps([doc.get("status"), doc.get("error_type"), result.get("error", "")[:40]])


# --- services --------------------------------------------------------------

def read_key(key_file):
    if key_file:
        return Path(key_file).expanduser().read_text().strip()
    if os.environ.get("OPENAI_API_KEY"):
        return os.environ["OPENAI_API_KEY"].strip()
    if DEFAULT_KEY_FILE.exists():
        return DEFAULT_KEY_FILE.read_text().strip()
    sys.exit(f"No API key: pass --key-file, set OPENAI_API_KEY, or create {DEFAULT_KEY_FILE}")


def write_data_dir(data_dir, prompt, schema, key, http_port, proxy_port, model):
    cfg_dir = data_dir / "config"
    cfg_dir.mkdir(parents=True, exist_ok=True)
    shutil.copy(prompt, cfg_dir / "system_prompt.md")
    shutil.copy(schema, cfg_dir / "json_schema.json")
    base = json.loads((REPO / "data/config/config.json").read_text())
    base["server"].update(port=http_port, proxy_url=f"http://localhost:{proxy_port}",
                          rosbridge_url="ws://127.0.0.1:1")
    base["proxy"]["port"] = proxy_port
    base["proxy"]["llm_config"].update(provider="openai", model=model,
                                       base_url="https://api.openai.com/v1/chat/completions",
                                       api_key=key, timeout_seconds=60)
    path = cfg_dir / "config.json"
    path.write_text(json.dumps(base, indent=2))
    path.chmod(0o600)


def start_llm_proxy(data_dir, proxy_port):
    for name in ("llm-proxy", "backend"):
        if not (BIN / name).exists():
            sys.exit(f"{BIN / name} not found, run ./run.sh --build-only first")
    logs = data_dir / "logs"
    logs.mkdir(exist_ok=True)
    proc = subprocess.Popen([BIN / "llm-proxy", "-dataDir", data_dir, "-httpPort", str(proxy_port)],
                            stdout=open(logs / "proxy.out", "w"), stderr=subprocess.STDOUT)
    for _ in range(50):
        try:
            socket.create_connection(("localhost", proxy_port), timeout=1).close()
            return [proc]
        except OSError:
            time.sleep(0.2)
    stop([proc])
    sys.exit(f"llm-proxy did not start, see {logs}")


def stop(procs):
    for p in procs:
        p.terminate()
    for p in procs:
        p.wait(timeout=10)


def assembled_prompt_tokens(prompt, schema, defaults):
    enc = tiktoken.get_encoding("o200k_base")
    text = Path(prompt).read_text().replace("{schema_template}", Path(schema).read_text())
    text = text.replace("{available_objects}", "\n".join("- " + o for o in defaults["objects"]))
    text = text.replace("{available_movements}", "\n".join("- " + o for o in defaults["movements"]))
    text = text.replace("{available_orientations}", "\n".join("- " + o for o in defaults["orientations"]))
    return len(enc.encode(text))


def eval_suite(cases, defaults):
    """cases.yaml as a backend -eval suite: no expectations, bench.py scores the results itself."""
    return {"environment": {}, "cases": [{
        "name": c["id"],
        "prompt": c["prompt"],
        "environment": {
            "objects": c.get("objects", defaults["objects"]),
            "movements": c.get("movements", defaults["movements"]),
            "orientations": c.get("orientations", defaults["orientations"]),
            "table_bounds": defaults["table_bounds"],
            "held_objects": c.get("held", defaults["held"]),
        },
    } for c in cases]}


def run_eval(data_dir, cases, defaults, tag):
    """Runs the cases once through backend -eval and returns {case id: result}."""
    suite = data_dir / f"suite-{tag}.json"
    results = data_dir / f"results-{tag}.jsonl"
    suite.write_text(json.dumps(eval_suite(cases, defaults)))
    subprocess.run([BIN / "backend", "-dataDir", data_dir, "-eval", "-evalJSON", results, suite],
                   stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if not results.exists():
        sys.exit(f"backend -eval wrote no results, see {data_dir / 'logs'}")
    return {r["name"]: r for r in map(json.loads, results.read_text().splitlines())}


def cmd_run(args):
    defaults, cases = load_cases(args.cases)
    if args.only:
        cases = [c for c in cases if c["id"] in args.only.split(",")]
    key = read_key(args.key_file)
    data_dir = WORK / args.name
    shutil.rmtree(data_dir, ignore_errors=True)
    write_data_dir(data_dir, args.prompt, args.schema, key, args.port, args.port + 1, args.model)
    procs = start_llm_proxy(data_dir, args.port + 1)
    enc = tiktoken.get_encoding("o200k_base")
    by_id = {c["id"]: c for c in cases}

    try:
        with ThreadPoolExecutor(args.workers) as pool:
            runs = list(pool.map(lambda rep: run_eval(data_dir, cases, defaults, f"rep{rep}"), range(args.reps)))
        # rate-limited calls are retried with backoff rather than scored as failures
        for attempt in range(5):
            limited = [(rep, cid) for rep, res in enumerate(runs) for cid, r in res.items()
                       if "status 429" in (r.get("error") or "")]
            if not limited:
                break
            time.sleep(10 * 2 ** attempt)
            for rep in {rep for rep, _ in limited}:
                retry = [by_id[cid] for r, cid in limited if r == rep]
                runs[rep].update(run_eval(data_dir, retry, defaults, f"rep{rep}-retry{attempt}"))
    finally:
        stop(procs)

    out = HERE / "results" / f"{args.name}.jsonl"
    out.parent.mkdir(exist_ok=True)
    with out.open("w") as f:
        meta = {"meta": True, "variant": args.name, "prompt_file": str(args.prompt), "model": args.model,
                "prompt_tokens": assembled_prompt_tokens(args.prompt, args.schema, defaults),
                "reps": args.reps, "cases": len(cases)}
        f.write(json.dumps(meta) + "\n")
        for rep, res in enumerate(runs):
            for case in cases:
                r = res.get(case["id"], {"error": "missing from eval results"})
                row = {"raw_output": r.get("raw_output", ""), "parsed": r.get("parsed"),
                       "error": r.get("error") or "", "latency_s": round(r.get("latency_s", 0), 3)}
                row["output_tokens"] = len(enc.encode(row["raw_output"]))
                ok, why = score(case, row)
                f.write(json.dumps({"variant": args.name, "id": case["id"], "category": case["category"],
                                    "rep": rep, "prompt": case["prompt"], "pass": ok, "reason": why,
                                    "sig": signature(row), **row}) + "\n")
    print(f"wrote {out}")


# --- report ----------------------------------------------------------------

def load_results(path):
    lines = [json.loads(l) for l in Path(path).read_text().splitlines()]
    return lines[0], lines[1:]


def summarize(meta, rows):
    by_case = defaultdict(list)
    for r in rows:
        by_case[r["id"]].append(r)
    cats = defaultdict(lambda: [0, 0])
    for r in rows:
        cats[r["category"]][0] += r["pass"]
        cats[r["category"]][1] += 1
    lat = sorted(r["latency_s"] for r in rows)
    out_tok = statistics.mean(r["output_tokens"] for r in rows)
    return {
        "variant": meta["variant"],
        "prompt_tokens": meta["prompt_tokens"],
        "runs": len(rows),
        "pass_rate": sum(r["pass"] for r in rows) / len(rows),
        "cases_all_pass": sum(all(x["pass"] for x in v) for v in by_case.values()) / len(by_case),
        "cases_consistent": sum(len({x["sig"] for x in v}) == 1 for v in by_case.values()) / len(by_case),
        "schema_invalid": sum("schema validation" in (r.get("error") or "") for r in rows),
        "transport_errors": sum(bool(r.get("transport_error")) or (bool(r.get("error")) and not r.get("raw_output")) for r in rows),
        "aggressive_refused": sum(r["category"] == "aggressive" and (r.get("parsed") or {}).get("status") == "error" for r in rows),
        "latency_p50": lat[len(lat) // 2],
        "latency_p95": lat[int(len(lat) * 0.95) - 1],
        "cost_per_1k_prompts": (meta["prompt_tokens"] * PRICE_IN + out_tok * PRICE_OUT) / 1000,
        "categories": {k: v[0] / v[1] for k, v in sorted(cats.items())},
        "by_case": by_case,
    }


def rescore(rows, cases_path):
    _, cases = load_cases(cases_path)
    by_id = {c["id"]: c for c in cases}
    for r in rows:
        if r["id"] in by_id:
            r["pass"], r["reason"] = score(by_id[r["id"]], r)
    return rows


def cmd_report(args):
    loaded = [load_results(p) for p in args.results]
    sums = [summarize(meta, rescore(rows, args.cases)) for meta, rows in loaded]
    names = [s["variant"] for s in sums]
    row = lambda label, vals: print(f"{label:<28}" + "".join(f"{v:>16}" for v in vals))
    pct = lambda x: f"{x * 100:.1f}%"
    row("", names)
    row("prompt tokens", [s["prompt_tokens"] for s in sums])
    row("runs", [s["runs"] for s in sums])
    row("pass rate (all runs)", [pct(s["pass_rate"]) for s in sums])
    row("cases passing every rep", [pct(s["cases_all_pass"]) for s in sums])
    row("cases fully consistent", [pct(s["cases_consistent"]) for s in sums])
    row("schema-invalid outputs", [s["schema_invalid"] for s in sums])
    row("transport/LLM errors", [s["transport_errors"] for s in sums])
    row("aggressive cases refused", [s["aggressive_refused"] for s in sums])
    row("latency p50 / p95 (s)", [f"{s['latency_p50']:.1f} / {s['latency_p95']:.1f}" for s in sums])
    row("est. USD per 1k prompts", [f"${s['cost_per_1k_prompts']:.2f}" for s in sums])
    print()
    for cat in sums[0]["categories"]:
        row(f"  {cat}", [pct(s["categories"].get(cat, 0)) for s in sums])

    print("\nFailing cases (passes/reps, most common failure):")
    ids = list(sums[0]["by_case"])
    for cid in ids:
        cells, any_fail = [], False
        for s in sums:
            v = s["by_case"].get(cid, [])
            n = sum(x["pass"] for x in v)
            any_fail |= n < len(v)
            fails = [x["reason"] for x in v if not x["pass"]]
            common = max(set(fails), key=fails.count) if fails else ""
            cells.append((n, len(v), common))
        if any_fail:
            print(f"  {cid} {sums[0]['by_case'][cid][0]['prompt']!r}")
            for name, (n, tot, why) in zip(names, cells):
                print(f"      {name:<12} {n}/{tot}  {why}")


def main():
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    r = sub.add_parser("run")
    r.add_argument("--name", required=True)
    r.add_argument("--prompt", default=str(REPO / "data/config/system_prompt.md"))
    r.add_argument("--schema", default=str(REPO / "data/config/json_schema.json"))
    r.add_argument("--key-file", help=f"default: $OPENAI_API_KEY, then {DEFAULT_KEY_FILE}")
    r.add_argument("--model", default="gpt-4o-mini")
    r.add_argument("--cases", default=str(HERE / "cases.yaml"))
    r.add_argument("--reps", type=int, default=5)
    r.add_argument("--workers", type=int, default=3)
    r.add_argument("--port", type=int, default=18080)
    r.add_argument("--only", help="comma-separated case ids")
    p = sub.add_parser("report")
    p.add_argument("results", nargs="+")
    p.add_argument("--cases", default=str(HERE / "cases.yaml"))
    args = ap.parse_args()
    cmd_run(args) if args.cmd == "run" else cmd_report(args)


if __name__ == "__main__":
    main()
