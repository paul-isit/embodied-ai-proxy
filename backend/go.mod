module embodied-ai-proxy/backend

go 1.26.5

require (
	embodied-ai-proxy/shared v0.0.0
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
)

require github.com/gorilla/websocket v1.5.3

replace embodied-ai-proxy/shared => ../shared
