from .ollama import OllamaAdapter
from .anthropic import AnthropicAdapter
from .openai import OpenAIAdapter
from .gemini import GeminiAdapter

ADAPTER_REGISTRY = {
    "ollama": OllamaAdapter,
    "anthropic": AnthropicAdapter,
    "openai": OpenAIAdapter,
    "gemini": GeminiAdapter,
}

def get_adapter(config):
    adapter_class = ADAPTER_REGISTRY.get(config.provider.lower())
    if not adapter_class:
        available = ", ".join(ADAPTER_REGISTRY.keys())
        raise ValueError(
            f"Unsupported provider: '{config.provider}'. "
            f"Available providers: {available}"
        )
    return adapter_class(config)