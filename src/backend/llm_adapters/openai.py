import json
import requests
from .base import BaseLLMAdapter

#API reference: https://developers.openai.com/api/docs
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

        data = self._post(self.config.base_url, payload, headers)
        return data["choices"][0]["message"]["content"].strip()