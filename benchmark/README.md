# Prompt benchmark

Measures how reliably the LLM turns natural-language commands into the right recipe, so changes to `data/config/system_prompt.md`, the schema or the model can be compared with numbers rather than by feel.

For each run it starts its own `llm-proxy` (port 18081 by default, with a config generated under `benchmark/.work/`), runs every case in `cases.yaml` through `backend -eval` several times, and scores the results from `-evalJSON`. `-eval` runs the full pipeline in-process (prompt, LLM, schema validation, name matching, missing-object retry) but never executes anything, so no ROS2 or robot is needed. `tests/*.json` (`./run.sh --eval`) checks single runs against fixed expectations; this benchmark adds repetitions, tolerant checks and side-by-side comparison of prompt versions.

## Setup

```bash
./run.sh --build-only
python3 -m venv .venv && .venv/bin/pip install -r benchmark/requirements.txt
```

The OpenAI key is read from `--key-file`, then `$OPENAI_API_KEY`, then `~/.config/embodied-ai-proxy/openai_key`. It is only written into the gitignored `benchmark/.work/`.

## Running

```bash
# baseline: the repo's current prompt
.venv/bin/python benchmark/bench.py run --name baseline

# a candidate prompt
.venv/bin/python benchmark/bench.py run --name trimmed --prompt path/to/trimmed_prompt.md

# hold-out set (not used for tuning)
.venv/bin/python benchmark/bench.py run --name holdout --cases benchmark/holdout.yaml

# compare
.venv/bin/python benchmark/bench.py report benchmark/results/baseline.jsonl benchmark/results/trimmed.jsonl
```

Options: `--reps` (default 5), `--model` (default `gpt-4o-mini`), `--schema`, `--only b01,p03`, `--workers` (parallel repetitions, default 3; more can hit OpenAI rate limits, which are retried with backoff), `--port` (the llm-proxy uses this + 1).

## Report

- **pass rate**: share of all runs that passed.
- **cases passing every rep**: cases that passed on all repetitions.
- **cases fully consistent**: cases whose output was identical across repetitions.
- **schema-invalid outputs**, **transport/LLM errors**.
- **aggressive cases refused**: should be 0. The research setup requires aggressive phrasing to map onto real actions, not be refused.
- **latency**, **prompt tokens** (o200k tokenizer) and **estimated cost** (gpt-4o-mini prices, see `PRICE_IN`/`PRICE_OUT`).
- Per-category pass rates, then every failing case with its most common failure reason.

## Cases

`cases.yaml` documents its own format. In short: an expected status, acceptable action sequences, and per-step parameter checks (exact, one-of, absent/present, `<x`, `>x`, `range:a:b`). Leading and trailing `home` steps are ignored unless the expected sequence includes them. Each case can override the object list.
