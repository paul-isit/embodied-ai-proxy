import requests
from .base import BaseLLMAdapter

class AnthropicAdapter(BaseLLMAdapter):
    def generate(self, prompt: str) -> str:
        payload = {
            "model": self.config.model,
            "max_tokens": self.config.max_tokens,
            "temperature": self.config.temperature,
            "messages": [
                {"role": "user", "content": prompt}
            ]
        }
        headers = {
            "x-api-key": self.config.api_key,
            "anthropic-version": "2023-06-01",
            "Content-Type": "application/json"
        }

        try:
            response = requests.post(
                self.config.base_url,
                json=payload,
                headers=headers,
                timeout=self.config.timeout_seconds
            )
            response.raise_for_status()
            return response.json()["content"][0]["text"].strip()

        except requests.exceptions.RequestException as e:
            raise RuntimeError(
                f"Failed to connect to Anthropic at {self.config.base_url}. Error: {e}"
            )