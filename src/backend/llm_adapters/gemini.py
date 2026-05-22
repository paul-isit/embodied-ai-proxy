import json
import requests
from urllib.parse import urljoin
from .base import BaseLLMAdapter


#API reference: https://ai.google.dev/api/generate-content
class GeminiAdapter(BaseLLMAdapter):

    def generate(self, prompt: str) -> str:
        # Gemini's URL structure is different, the model and key go in the URL itself
        base = self.config.base_url.rstrip("/")
        url = f"{base}/{self.config.model}:generateContent"
        params = {"key": self.config.api_key}

        payload = {
            "contents": [
                {
                    "parts": [
                        {"text": prompt}
                    ]
                }
            ],
            "generationConfig": {
                "maxOutputTokens": self.config.max_tokens,
                "temperature": self.config.temperature
            }
        }
        headers = {
            "Content-Type": "application/json"
        }

        data = self._post(url, payload, headers, params=params)
        return data["candidates"][0]["content"]["parts"][0]["text"].strip()
    