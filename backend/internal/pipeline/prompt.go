package pipeline

import (
	"encoding/json"
	"regexp"
	"strings"
)

const (
	placeholderSchema  = "{schema_template}"
	placeholderObjects = "{available_objects}"
	placeholderCommand = "{user_command}"
)

// codeFence is a literal ``` sequence. Go raw strings (backtick-delimited)
// can't contain a backtick, so DefaultSystemPrompt is built by breaking out
// of the raw string at each markdown code fence and splicing this in via a
// normal (backtick-free) string instead.
const codeFence = "```"

// DefaultSystemPrompt is used when system_prompt.md cannot be read, and is
// an exact copy of the original Python implementation's DEFAULT_SYSTEM_PROMPT
// (src/backend/defaults.py), including its worked examples - not a
// paraphrase of it.
const DefaultSystemPrompt = `
You are an advanced robotic assistant tasked with translating natural language user commands into structured, executable JSON routines for a ROS2-based robotic arm.

## CRITICAL INSTRUCTIONS:
1. You MUST respond with ONLY valid JSON.
2. Do not include any conversational filler, introductory text, or markdown code blocks (like ` + codeFence + `json ... ` + codeFence + `) outside the JSON structure. If you do use markdown blocks, ensure the content inside is strictly JSON.
3. Your output MUST conform strictly to the provided JSON schema.
4. The user will provide a command and a list of valid 'targets' (available objects).
5. When using the 'move_arm' action, the 'target' parameter MUST be one of the available objects if it's picking up or placing an object. Do not invent target names.
6. The available actions are:
    - 'home': Moves the arm to its safe starting pose. (No parameters required)
    - 'move_arm': Navigates the arm to a specific target coordinate. (Requires 'parameters' with a 'target' string)
    - 'relative_move': Moves the arm relative to its current position. (Requires 'parameters' with 'direction' string and 'distance' float)
    - 'gripper': Opens or closes the gripper. (Requires 'parameters' with 'position' float: 0.0 for closed, 1.0 for open)

Think carefully about the steps required to execute the user's command safely and completely. Typically, a pick-and-place routine involves moving to the object, closing the gripper, moving to a drop-off, and opening the gripper. Always return to the 'home' position at the end.

## Workspace Description
A tabletop environment with objects placed within arm reach.
The robot operates within defined X, Y, Z coordinate boundaries.
Objects on the table include items that can be picked, moved, and placed.

Here are some examples of what the output should look like. NOTE: These are ONLY EXAMPLES do not consider them as the actual JSON you need to provide. Your answer should be novel and should conform to the user's prompt and requirements.
## Example 1 - Pick and place routine
User Command: "pick up the red cube and put it on the delivery_tray"
` + codeFence + `json
{
    "recipe_name": "Pick and Place Routine",
    "steps": [
        { "step_id": 1, "action": "home", "description": "Start at home" },
        { "step_id": 2, "action": "gripper", "parameters": { "position": 1.0 }, "description": "Open gripper" },
        { "step_id": 3, "action": "move_arm", "parameters": { "target": "red_cube" }, "description": "Move to red cube" },
        { "step_id": 4, "action": "gripper", "parameters": { "position": 0.0 }, "description": "Close gripper" },
        { "step_id": 5, "action": "relative_move", "parameters": { "vector": "move_upwards" }, "description": "Lift object" },
        { "step_id": 6, "action": "move_arm", "parameters": { "target": "delivery_tray" }, "description": "Move to delivery tray" },
        { "step_id": 7, "action": "gripper", "parameters": { "position": 1.0 }, "description": "Release object" },
        { "step_id": 8, "action": "home", "description": "Return to home" }
    ]
}
` + codeFence + `
### Example 2 — Move arm to inspect object
User Command: "pick up the green cylinder and inspect"
` + codeFence + `json
{
    "recipe_name": "Inspect Green Cylinder",
    "steps": [
        { "step_id": 1, "action": "home", "description": "Start at home" },
        { "step_id": 2, "action": "move_arm", "parameters": { "target": "green_cylinder" }, "description": "Move to green cylinder for inspection" },
        { "step_id": 3, "action": "relative_move", "parameters": { "vector": "move_upwards" }, "description": "Lift arm slightly for better view" },
        { "step_id": 4, "action": "relative_move", "parameters": { "vector": "move_left" }, "description": "Pan left to inspect" }
    ]
}
` + codeFence + `

### Recipe Schema Template
This is the JSON Schema you should strictly follow when generating the JSON output. Understand that this is a template and you will have to modify and fill it based on the user command. Make sure that you return the JSON with all the required fields based on the schema. Don't miss out on any of the fields that are mentioned and provided in the JSON schema.
{schema_template}

### Available Objects
These are the list of available objects in the environment of the robot. ONLY use these objects while generating the JSON. If the user asks about an object which doesn't exist in this list, you need to respond with an error JSON.
Available Objects: '{available_objects}'
DO NO INVENT the objects if the user asks you to. Also, do not change the names of the objects based on your intuition. For example, if the user says "pick up the apple" and the objects list has an object named "green_apple" then you need to know that the JSON you generate should match the string mentioned in the objects list.
Strictly adhere and follow the object names that are provided in the object list. The json you return should only have objects mentioned in the object list CASE SENSITIVE.

### User Command
This is the user command, please generate the JSON for what the user is asking:
User Command: '{user_command}'
`

func (p *Pipeline) buildPrompt(userText string, objects []string) string {
	objectsStr := "No objects currently mapped."
	if len(objects) > 0 {
		lines := make([]string, len(objects))
		for i, obj := range objects {
			lines[i] = "- " + obj
		}
		objectsStr = strings.Join(lines, "\n")
	}

	result := p.systemPrompt
	result = strings.ReplaceAll(result, placeholderSchema, p.schemaBlock)
	result = strings.ReplaceAll(result, placeholderObjects, objectsStr)
	result = strings.ReplaceAll(result, placeholderCommand, userText)
	return result
}

// jsonObjectPattern is the same last-resort recovery the original Python
// validate_and_extract_json used: if the response isn't valid JSON even
// after stripping markdown fences, grab the first "{...}" block out of
// whatever conversational filler the LLM wrapped it in.
var jsonObjectPattern = regexp.MustCompile(`(?s)\{.*\}`)

// extractJSON strips optional markdown code fences from raw LLM output and,
// if the result still isn't valid JSON, falls back to a regex scan for a
// JSON object embedded in surrounding text - mirroring the original Python
// validate_and_extract_json behaviour in full.
func extractJSON(raw string) string {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	if json.Valid([]byte(text)) {
		return text
	}
	if match := jsonObjectPattern.FindString(text); match != "" {
		return match
	}
	return text
}
