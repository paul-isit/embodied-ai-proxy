# Evaluate a language model

An evaluation runs prompts through the command pipeline without the robot and compares each result with the outcomes that a test suite expects. Use it to compare language models and to check a change of the prompt or the schema. The steps of the pipeline are described in [Command pipeline](../technical/command-pipeline.md).

## Prerequisites

- The proxy is built, and `config.json` names the model to test. See [Configuration](../installation/configuration.md).
- The middleware does not need to run. The suite provides the objects, movements and orientations.

## Run a suite

Run the suites with `run.sh`. The script starts the LLM proxy, runs the backend in evaluation mode and stops the LLM proxy afterwards:

```bash
./run.sh --build --eval tests/*.json
```

`--eval` must be the last option. The exit code is 0 when every case passes and 1 otherwise.

To write the result of each case to a file, run the backend directly while the LLM proxy runs. Put `-evalJSON` before `-eval`, because every argument after `-eval` is a suite file:

```bash
bin/backend -dataDir data -evalJSON results.jsonl -eval tests/basic.json
```

The report has this form. The first line names the provider and the model of `config.json`. A failed case lists what it expected and what it got:

```text
Evaluating <provider> · <model>

tests/basic.json · 8 cases

  PASS go home                            1.3s
  PASS pick and place                     2.0s
  PASS typo in object name                1.3s
  PASS push in a direction                1.0s
  FAIL orientation on move_arm            1.0s
      prompt  move to the water bottle pointing straight down
      want    home → move_arm(orientation=top_down, target=water_bottle)
              ↳ steps[1].parameters.orientation: want "top_down", got "side_level"
      or      move_arm(orientation=top_down, target=water_bottle)
              ↳ steps: want 1 items, got 2
      got     home → move_arm(orientation=side_level, target=water_bottle)

  PASS relative move without orientation  2.1s
  PASS unknown object                     1.4s
  PASS free-form command is schema-valid  1.6s

────────────────────────────────────────────────
  7 passed · 1 failed · 8 cases · 11.8s
  FAIL orientation on move_arm
```

| Line of a failed case | Meaning |
|---|---|
| `prompt` | The prompt of the case. |
| `error` | The pipeline failed, for example because the model did not answer or the answer was invalid. A line `raw` shows the first 400 characters of the answer. |
| `want` | The first acceptable outcome as a chain of steps, written `action(parameter=value)`. The line below names the first difference. |
| `or` | Each further acceptable outcome, with its first difference. |
| `got` | The result of the pipeline. |

The prompts and the answers of the model are written to `data/logs/eval.log`.

## Write a suite

A suite is a JSON file. The environment replaces the middleware: the model is told these names, and nothing is read from the robot.

```json
{
  "environment": {
    "objects": ["blue_cube", "delivery_tray"],
    "movements": ["move_upwards"],
    "orientations": ["top_down"],
    "held_objects": []
  },
  "cases": [
    {
      "name": "pick and place",
      "prompt": "put the blue cube on the tray",
      "expect": [
        { "status": "success", "steps": [
          { "action": "pickup", "parameters": { "target": "blue_cube" } },
          { "action": "dropoff", "parameters": { "destination": "delivery_tray" } },
          { "action": "home" }
        ]}
      ]
    },
    {
      "name": "unknown object",
      "prompt": "pick up the red sphere",
      "expect": [{ "status": "error", "error_type": "missing_object" }]
    }
  ]
}
```

| Key | Level | Description |
|---|---|---|
| `environment` | Suite, case | The names that the model is told. A case can have its own `environment`, which replaces the one of the suite. |
| `environment.objects`, `movements`, `orientations` | | Lists of names. |
| `environment.table_bounds` | | `available`, `x_min`, `x_max`, `y_min` and `y_max`, in metres. |
| `environment.held_objects` | | The names of the held objects. `[]` means that nothing is held. Without the key, the model is told that it is unknown. |
| `cases` | Suite | The list of cases. |
| `name` | Case | The name in the report. The default is `case <n>`. |
| `prompt` | Case | Required. The command. |
| `expect` | Case | The acceptable outcomes. |

A key that is not in the table stops the load of the suite with `json: unknown field "<name>"`. A case without a prompt stops it with `case <n> has no prompt`.

The repository contains `tests/basic.json`. Its eight cases cover a bare `home`, pick and place, a misspelled object name, push by direction, an orientation, a relative move, an unknown object and a command with no matching action.

## Match the result

- A case passes when one entry of `expect` matches. A case without `expect` passes when the pipeline returns a result that satisfies the schema: a recipe or an error document.
- An expectation names only the keys to compare. `step_id`, `description` and `recipe_name` can be left out. A step without `action` matches any action.
- `null` means that the key must be absent from the result.
- Arrays match item by item and must have the same length. The `steps` of the result must match in order and in number.
- Numbers compare by value.
- The comparison uses the result after name resolution. A result with `blu cube` counts as `blue_cube`.
- An expectation can describe a refusal: `{ "status": "error", "error_type": "missing_object" }`. The error types are `missing_object`, `invalid_command` and `safety_violation`.
- A pipeline error fails the case.

## Read the results file

`-evalJSON` writes one JSON object per case, one per line:

| Key | Description |
|---|---|
| `suite` | The path of the suite file. |
| `name` | The name of the case. |
| `prompt` | The prompt. |
| `passed` | `true` or `false`. |
| `raw_output` | The answer of the model before name resolution. Left out when there is no answer. |
| `parsed` | The recipe or error document after name resolution. Left out when the pipeline failed. |
| `error` | The error of the pipeline. Left out when there is none. |
| `latency_s` | The seconds from the start of the case to its result. |

For the case `typo in object name`, `raw_output` contains `"target": "blu cube"` and `parsed` contains `"target": "blue_cube"`.
