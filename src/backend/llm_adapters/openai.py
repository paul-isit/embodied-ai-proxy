import requests
from .base import BaseLLMAdapter

class OpenAIAdapter(BaseLLMAdapter):
    def generate(self, prompt: str) -> str:
        payload = {
            "model": self.config.model,
            "messages": [
                {"role": "user", "content": prompt}
            ],
            "max_tokens": self.config.max_tokens,
            "temperature": self.config.temperature
        }
        headers = {
            "Authorization": f"Bearer {self.config.api_key}",
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
            return response.json()["choices"][0]["message"]["content"].strip()

        except requests.exceptions.RequestException as e:
            raise RuntimeError(
                f"Failed to connect to OpenAI at {self.config.base_url}. Error: {e}"
            )