import requests
from .base import BaseLLMAdapter

class GeminiAdapter(BaseLLMAdapter):
    def generate(self, prompt: str) -> str:
        # Gemini's URL structure is different - the model and key go in the URL itself
        url = f"{self.config.base_url}/{self.config.model}:generateContent?key={self.config.api_key}"

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
                "temperature": self.config.temperature,
                "responseMimeType": "application/json" 
            }
        }
        headers = {
            "Content-Type": "application/json"
        }

        try:
            response = requests.post(
                url,
                json=payload,
                headers=headers,
                timeout=self.config.timeout_seconds
            )
            response.raise_for_status()
            raw = response.json()["candidates"][0]["content"]["parts"][0]["text"].strip()
            return raw

        except requests.exceptions.RequestException as e:
            raise RuntimeError(
                f"Failed to connect to Gemini at {self.config.base_url}. Error: {e}"
            )