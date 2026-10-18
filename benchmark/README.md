# Prompt benchmark

Measures how reliably the LLM turns natural-language commands into the right recipe, so changes to `data/config/system_prompt.md`, the schema or the model can be compared with numbers rather than by feel.

For each run it starts its own `llm-proxy` and `backend` (ports 18080/18081 by default, with a config generated under `benchmark/.work/`), sends every case in `cases.yaml` to `POST /api/prompt` several times, and scores the results. `/api/prompt` runs the full pipeline (prompt, LLM, schema validation, name matching, missing-object retry) but never executes anything, so no ROS2 or robot is needed.

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

# compare
.venv/bin/python benchmark/bench.py report benchmark/results/baseline.jsonl benchmark/results/trimmed.jsonl
```

Options: `--reps` (default 5), `--model` (default `gpt-4o-mini`), `--schema`, `--only b01,p03`, `--workers` (default 3; more can hit OpenAI rate limits, which are retried with backoff), `--port`.

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
