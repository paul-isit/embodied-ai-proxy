from .ollama import OllamaAdapter
from .anthropic import AnthropicAdapter
from .openai import OpenAIAdapter
from .gemini import GeminiAdapter
from src.backend.llm_config import LLMConfig
from .base import BaseLLMAdapter

LLM_REGISTRY = {
    "ollama": OllamaAdapter,
    "anthropic": AnthropicAdapter,
    "openai": OpenAIAdapter,
    "gemini": GeminiAdapter,
}

def get_adapter(config: LLMConfig) -> BaseLLMAdapter:
    adapter_class = LLM_REGISTRY.get(config.provider.lower())
    if not adapter_class:
        available = ", ".join(LLM_REGISTRY.keys())
        raise ValueError(
            f"Unsupported provider: '{config.provider}'. "
            f"Available providers: {available}"
        )
    return adapter_class(config)