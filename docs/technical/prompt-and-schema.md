# Prompt and schema

The backend builds the prompt for each command from a template, the recipe schema, the scene and the command. The same schema validates the answer of the model. Both files are in `data/config/`. The backend reads them at start.

## Prompt template

`system_prompt.md` is the template. The backend replaces these placeholders:

| Placeholder | Replaced with |
|---|---|
| `{schema_template}` | The contents of `json_schema.json`, indented by two spaces. |
| `{available_objects}` | The object names, one per line. |
| `{available_movements}` | The movement names, one per line. |
| `{available_orientations}` | The orientation names, one per line. |
| `{table_bounds}` | A sentence with the range of x and y of the table, in metres. |
| `{held_objects}` | The names of the held objects, one per line. |
| `{user_command}` | The text of the command. |

A name list is shown as `- <name>` lines. Where the information is missing, the prompt shows a fixed text:

- `{available_objects}`: `No objects currently mapped.`
- `{available_movements}`: `No named relative movements are currently mapped.`
- `{available_orientations}`: `No named orientation presets are currently mapped.`
- `{table_bounds}`: `No table boundary information is currently available.`
- `{held_objects}`: `Nothing is currently held.` when nothing is held, and `Unknown - the arm may still be holding an object from an earlier command.` when the backend could not read it.

The shipped template has these sections: `CRITICAL OUTPUT INSTRUCTIONS`, `Environment`, `Available Actions`, `Reasoning About Parameters`, `Sequencing Rules`, `Object and Movement Names`, `Examples` with 18 worked examples, `Recipe Schema Template`, and one section for each remaining placeholder.

- The examples use their own object names, such as `red_cube` and `glass_vase`. They show the structure of an answer and do not describe the scene.
- With the shipped files and the six default objects, the prompt is 62 KB: 31 KB of template and 31 KB of schema.
- If `system_prompt.md` is missing, the backend uses a built-in copy of the shipped template and logs `system_prompt.md not found`.

## Recipe schema

`json_schema.json` is a JSON Schema of the draft 2020-12. An answer is one of two documents. No other keys are allowed.

| Document | Keys |
|---|---|
| Success | `status` with the value `success`, `recipe_name` (a non-empty string) and `steps` (a list with at least one step). |
| Error | `status` with the value `error`, `error_type` and `message` (a non-empty string). |

The value of `error_type` is `missing_object`, `invalid_command` or `safety_violation`. The schema describes an error document as: an object is missing, an object is ambiguous, or the command violates the constraints of the environment. The prompt shows `missing_object` and `invalid_command` with one example each. It does not describe `safety_violation`.

Each step has `step_id` (an integer from 1), `action`, `description` and `parameters`. `parameters` is required for every action except `home`. These are the actions with their parameters and constraints:

- `home`: `speed`.
- `move_arm`: `target` (required), `orientation`, `speed`.
- `relative_move`: `vector` (required), `speed`.
- `gripper`: `position` (required), 0.0 to 1.0.
- `pickup`: `target` (required), `grasp_style` (`auto`, `top` or `side`).
- `dropoff`: `target`, `destination`, `direction`, `distance`, `place_offset`. `direction` needs `distance`. `place_offset` is at most 0.12.
- `pour`: `target`, `destination`, `direction`, `distance`, `lift_height`, `tilt_angle`, `duration`, `speed`. `tilt_angle` is -3.15 to 3.15. `duration` is 0 or more. `parameters` can be empty.
- `thrust`: `target`, `destination`, `direction`, `distance`, `lift_height`, `speed`. `destination` or `direction` is required.
- `push`: `target` (required), `destination`, `direction`, `distance`, `close_position` (0.0 to 1.0), `speed`. `destination` or `direction` is required.
- `throw`: `target`, `destination`, `direction`, `distance`, `open_position` (0.0 to 1.0), `speed`. `destination` or `direction` is required.

Constraints of all actions:

| Parameter | Constraint |
|---|---|
| `target`, `destination`, `vector`, `orientation` | A non-empty string. The backend resolves it with the live names. See [Name resolution](command-pipeline.md#name-resolution). |
| `speed` | 0.0 to 1.0. |
| `direction` | `forward`, `backward`, `left` or `right`. |
| `distance`, `lift_height` | 0 or more, in metres. |

The meaning of each parameter is described in the [Command reference](https://jeremy-allan.github.io/ROS2-middleware/user-guide/command-reference/) of the middleware. The middleware does not use `close_position`.

## Change the prompt or the schema

- Edit the files in `data/config/` and restart the backend.
- The prompt must contain `{schema_template}` and `{user_command}`. Without them the model does not see the schema or the command.
- The names of the objects, movements and orientations come from the middleware. A change of the scene needs no change here.
- A new action or parameter in the schema also needs support in the middleware. The middleware does not validate recipes against this schema.
- Test the change with a suite. See [Evaluate a language model](../user-guide/evaluation.md).
