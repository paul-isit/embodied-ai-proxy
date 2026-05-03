import json
import logging
import requests
import re
from pathlib import Path
from pydantic import BaseModel
from typing import List, Dict, Optional, Any
from src.backend.defaults import DEFAULT_SYSTEM_PROMPT

class LLMConfig(BaseModel):
    provider: str
    model: str
    base_url: str
    api_key: str
    max_tokens: int
    temperature: float
    timeout_seconds: int

class StepSchema(BaseModel):
    step_id: int
    action: str
    description: str
    parameters: Optional[Dict[str, Any]] = None

class JSONSchema(BaseModel):
    recipe_name: str
    steps: List[StepSchema]

class SystemConfig(BaseModel):
    llm: LLMConfig
    json_schema: JSONSchema
    system_prompt: str

class LLMProxy:
    def __init__(self, config_path: str, bridge_url: str = "http://localhost:9090"):
        self.bridge_url = bridge_url
        self.system_config = self._load_config(config_path)
        self._verify_ollama_connection()

    def _verify_ollama_connection(self):
        url = f"{self.system_config.llm.base_url}/api/tags"
        try:
            r = requests.get(url, timeout=5)
            r.raise_for_status()
            print("Ollama connected")
        except Exception as e:
            raise RuntimeError(f"Ollama unreachable: {e}")

    def _load_json_file(self, path: Path, filename: str) -> dict:
        with open(path / filename, "r") as f:
            return json.load(f)

    def _load_config(self, config_path):
        path = Path(config_path)

        llm_config = self._load_json_file(path, "llm_config.json")
        json_schema = self._load_json_file(path, "json_schema.json")

        system_prompt_path = path / "system_prompt.md"

        system_prompt = (
            system_prompt_path.read_text()
            if system_prompt_path.exists()
            else DEFAULT_SYSTEM_PROMPT
        )

        return SystemConfig(
            llm=LLMConfig(**llm_config),
            json_schema=JSONSchema(**json_schema),
            system_prompt=system_prompt
        )

    def get_environment_context(self):
        return ["apple", "banana"]

    def build_prompt(self, user_text: str, objects: List[str]) -> str:
        return f"""
SYSTEM:
{self.system_config.system_prompt}

AVAILABLE OBJECTS:
{objects}

USER:
{user_text}

RULE:
Return ONLY valid JSON in this format:
{{"action": "string", "target": "string"}}
""".strip()

    def generate_llm_response(self, prompt: str) -> str:
        url = f"{self.system_config.llm.base_url}/api/generate"

        payload = {
            "model": self.system_config.llm.model,
            "prompt": prompt,
            "stream": False,
            "format": "json",
            "options": {
                "temperature": self.system_config.llm.temperature,
                "num_predict": self.system_config.llm.max_tokens
            }
        }

        r = requests.post(
            url,
            json=payload,
            timeout=self.system_config.llm.timeout_seconds
        )

        r.raise_for_status()
        return r.json().get("response", "")

    def validate_and_extract_json(self, text: str) -> dict:
        text = text.strip().replace("```json", "").replace("```", "")

        try:
            return json.loads(text)
        except json.JSONDecodeError:
            match = re.search(r"\{.*\}", text, re.DOTALL)
            if not match:
                raise ValueError(f"No JSON found: {text}")
            return json.loads(match.group(0))

    def validate_schema(self, obj: dict):
        if "action" not in obj or "target" not in obj:
            raise ValueError("Invalid schema")
        return True

    def generate(self, user_prompt: str, available_objects: list[str]):
        prompt = self.build_prompt(user_prompt, available_objects)

        raw = self.generate_llm_response(prompt)
        parsed = self.validate_and_extract_json(raw)

        return {
            "raw_output": raw,
            "parsed": parsed
        }
