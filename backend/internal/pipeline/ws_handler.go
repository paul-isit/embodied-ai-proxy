package pipeline

import (
	"context"
	"embodied-ai-proxy/backend/internal/websocket"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const (
	defaultLLMTimeout       = 60 * time.Second
	defaultExecutionTimeout = 120 * time.Second
)

// HandlePrompt implements websocket.PromptHandler: it runs the pipeline
// using the workspace object list from the robot bridge, delivers the action recipe
// to the connected client upon successful robot execution, and dispatches it directly to the robot.
func (p *Pipeline) HandlePrompt(ctx context.Context, userText string) {
	if p.bridge != nil && !p.bridge.IsConnected() {
		log.Printf("[Pipeline] rejecting command %q: no ROS 2 bridge connected", userText)
		p.sendLog("error", "cannot execute commands: no ROS 2 bridge connected")
		return
	}

	var availableObjs []string
	if p.bridge != nil {
		availableObjs = p.bridge.GetAvailableObjects()
	}

	llmCtx, llmCancel := context.WithTimeout(ctx, defaultLLMTimeout)
	defer llmCancel()

	result := p.Run(llmCtx, userText, availableObjs)
	if ctx.Err() != nil {
		log.Printf("[Pipeline] command %q aborted: context canceled/timed out: %v", userText, ctx.Err())
		return
	}
	if result.Error != "" {
		p.sendLog("error", result.Error)
		return
	}

	if recipeStatus(result.Doc) == "success" {
		if p.bridge != nil {
			execCtx, execCancel := context.WithTimeout(ctx, defaultExecutionTimeout)
			defer execCancel()

			if err := p.bridge.ExecuteRecipe(execCtx, result.Parsed); err != nil {
				log.Printf("[Pipeline] command %q: robot recipe execution failed: %v", userText, err)
				p.sendLog("error", fmt.Sprintf("robot recipe execution failed: %v", err))
				return
			}
			p.hub.SendToClient(websocket.Envelope{Type: websocket.TypeActionRecipe, Payload: result.Parsed})
			p.sendLog("info", fmt.Sprintf("recipe %q executed successfully by robot", recipeName(result.Doc)))
		} else {
			p.hub.SendToClient(websocket.Envelope{Type: websocket.TypeActionRecipe, Payload: result.Parsed})
		}
	} else {
		p.hub.SendToClient(websocket.Envelope{Type: websocket.TypeLogEvent, Payload: result.Parsed})
	}
}

func (p *Pipeline) sendLog(level, message string) {
	payload, err := json.Marshal(map[string]string{"level": level, "message": message})
	if err != nil {
		return
	}
	p.hub.SendToClient(websocket.Envelope{Type: websocket.TypeLogEvent, Payload: payload})
}
