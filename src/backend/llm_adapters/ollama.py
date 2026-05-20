import requests
from .base import BaseLLMAdapter

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

        try:
            response = requests.post(
                self.config.base_url,
                json=payload,
                timeout=self.config.timeout_seconds
            )
            response.raise_for_status()
            return response.json().get("response", "").strip()

        except requests.exceptions.RequestException as e:
            raise RuntimeError(
                f"Failed to connect to Ollama at {self.config.base_url}. Error: {e}"
            )