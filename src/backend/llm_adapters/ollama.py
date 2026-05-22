import json
import requests
from .base import BaseLLMAdapter

#API reference: https://github.com/ollama/ollama/blob/main/docs/api.md
class OllamaAdapter(BaseLLMAdapter):

    def generate(self, prompt: str) -> str:
        payload = {
            "model": self.config.model,
            "prompt": prompt,
            "stream": False,
            "format": "json",
            "options": {
                "temperature": self.config.temperature,
                "num_predict": self.config.max_tokens
            }
        }

        data = self._post(self.config.base_url, payload, headers={})
        return data.get("response", "").strip()