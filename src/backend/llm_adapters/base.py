from src.backend.llm_config import LLMConfig
import json 
import requests

class BaseLLMAdapter:
    def __init__(self, config: LLMConfig):
        self.config = config

    def generate(self, prompt: str) -> str:
        raise NotImplementedError(
            f"'{self.__class__.__name__}' must implement a generate() method."
        )
    
    def _post(self, url: str, payload: dict, headers: dict, params: dict = None) -> dict:
        """
        Protected method that handles the HTTP POST request and error handling.
        Reused by all adapter subclasses.
        """
        try:
            response = requests.post(
                url,
                json=payload,
                headers=headers,
                params=params,
                timeout=self.config.timeout_seconds
            )
            response.raise_for_status()
            return response.json()

        except requests.exceptions.RequestException as e:
            raise RuntimeError(
                f"Failed to connect to {self.config.provider} at {url}. Error: {e}"
            )
        except json.JSONDecodeError as e:
            raise RuntimeError(f"{self.config.provider} returned invalid JSON: {e}")
        except KeyError as e:
            raise RuntimeError(f"Unexpected {self.config.provider} response structure, missing key: {e}")
        except IndexError as e:
            raise RuntimeError(f"Unexpected {self.config.provider} response structure, empty list: {e}")
        except ValueError as e:
            raise RuntimeError(f"Failed to decode {self.config.provider} response: {e}")