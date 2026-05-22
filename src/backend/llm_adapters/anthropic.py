import json
import requests
from .base import BaseLLMAdapter

#API reference: https://platform.claude.com/docs/en/api/messages/create

ANTHROPIC_API_VERSION = "2023-06-01"
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
            "anthropic-version": "ANTHROPIC_API_VERSION",
            "Content-Type": "application/json"
        }

        data = self._post(self.config.base_url, payload, headers)
        return data["content"][0]["text"].strip()