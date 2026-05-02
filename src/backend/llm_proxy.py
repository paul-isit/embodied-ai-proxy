import json
import logging
from pathlib import Path
from pydantic import BaseModel, ValidationError
from typing import List, Dict, Optional, Any

DEFAULT_SYSTEM_PROMPT = "You are a helpful robot assistant. Output strictly in JSON."

# --- PYDANTIC MODELS ---

class LLMConfig(BaseModel):
    provider: str 
    model: str 
    base_url: str 
    api_key: str 
    max_tokens: int 
    temperature: float
    timeout_seconds: int

    # Explicitly ignore any unexpected fields in the JSON
    class Config:
        extra = "ignore"

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


# --- MAIN PROXY CLASS ---

class LLMProxy:
    # Fix: config_path is now mandatory, no default empty string
    def __init__(self, config_path: str, bridge_url: str = "http://localhost:9090"):
        self.bridge_url = bridge_url
        self.system_config = self._load_config(config_path)

    def _load_json_file(self, folder_path: Path, filename: str) -> dict:
        """Safely loads a JSON file using pathlib and try/except blocks."""
        file_path = folder_path / filename
        try:
            with open(file_path, "r") as f:
                return json.load(f)
        except FileNotFoundError:
            logging.error(f"Missing file: {file_path}. Please check your config directory.")
            raise
        except json.JSONDecodeError as e:
            logging.error(f"Malformed JSON in {filename}: {e}")
            raise

    def _load_config(self, config_path: str) -> SystemConfig:
        # Fix: Strictly use the provided path, no fallback to "."
        target_dir = Path(config_path)
        
        # Load the raw dictionaries using the safe helper method
        raw_llm = self._load_json_file(target_dir, "llm_config.json")
        raw_schema = self._load_json_file(target_dir, "json_schema.json")
        
        system_prompt_path = target_dir / "system_prompt.md"
        
        # Safe loading for the markdown text file
        try:
            if system_prompt_path.exists():
                with open(system_prompt_path, "r") as f:
                    system_prompt_text = f.read()
            else:
                logging.warning(f"{system_prompt_path} not found. Using default prompt.")
                system_prompt_text = "You are a helpful robot assistant. Output strictly in JSON."
        except Exception as e:
            logging.error(f"Failed to read system prompt: {e}")
            system_prompt_text = "You are a helpful robot assistant. Output strictly in JSON."

        # Pass dictionaries into Pydantic models. 
        try:
            return SystemConfig(
                llm=LLMConfig(**raw_llm),
                json_schema=JSONSchema(**raw_schema),
                system_prompt=system_prompt_text
            )
        except ValidationError as e:
            logging.critical(f"Config Validation Error! Your JSON files do not match the expected types: {e}")
            raise

    def get_environment_context(self) -> list[str]:
        return ['blue_block', 'red_cube', 'table', 'window']

    def build_prompt(self, user_text: str, available_objects: List[str]) -> str:
        # Dumps the Pydantic model back to a clean JSON string for the prompt
        schema_block = json.dumps(self.system_config.json_schema.model_dump(), indent=2)

        return (
            f"{self.system_config.system_prompt}\n\n"
            f"### Expected Output Schema\n{schema_block}\n\n"
            f"### Available Objects\n{available_objects}\n\n"
            f"### User Command\n{user_text}\n\n"
            f"Task: Generate a valid JSON recipe."
        )

    def process_user_request(self, user_text: str) -> dict:
        try:
            available_objects = self.get_environment_context()
            prompt = self.build_prompt(user_text, available_objects)
            
            return {
                "prompt": prompt,
                "json": None,
                "execution_result": "test_mode",
                "error": None
            }
        except Exception as e:
            return {
                "prompt": f"Failed to build prompt for: {user_text}",
                "json": None,
                "execution_result": "failure",
                "error": str(e)
            }
