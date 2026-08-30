package pipeline

import (
	"context"
	"embodied-ai-proxy/backend/internal/websocket"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// SetAvailableObjects updates the cached workspace object list, typically
// from a status_update pushed by the ROS 2 bridge (see HandleBridgeStatus).
func (p *Pipeline) SetAvailableObjects(objects []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.objects = objects
}

func (p *Pipeline) availableObjects() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.objects
}

// HandleBridgeStatus implements websocket.StatusHandler: it inspects a
// status_update from the bridge for an object_list field and caches it.
func (p *Pipeline) HandleBridgeStatus(env websocket.Envelope) {
	var payload struct {
		ObjectList []string `json:"object_list"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return
	}
	if payload.ObjectList != nil {
		p.SetAvailableObjects(payload.ObjectList)
	}
}

// HandlePrompt implements websocket.PromptHandler: it runs the pipeline
// using the cached workspace object list and sends the outcome to the
// connected client.
func (p *Pipeline) HandlePrompt(userText string) {
	if !p.hub.BridgeConnected() {
		log.Printf("[Pipeline] rejecting command %q: no ROS 2 bridge connected", userText)
		p.sendLog("error", "cannot execute commands: no ROS 2 bridge connected")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result := p.Run(ctx, userText, p.availableObjects())
	if result.Error != "" {
		p.sendLog("error", result.Error)
		return
	}

	if recipeStatus(result.Doc) == "success" {
		if err := p.hub.SendToBridge(websocket.Envelope{Type: websocket.TypeActionRecipe, Payload: result.Parsed}); err != nil {
			log.Printf("[Pipeline] command %q: failed to dispatch action recipe to bridge: %v", userText, err)
			p.sendLog("error", fmt.Sprintf("failed to dispatch action recipe to the ROS 2 bridge: %v", err))
			return
		}
		p.hub.SendToClient(websocket.Envelope{Type: websocket.TypeActionRecipe, Payload: result.Parsed})
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
