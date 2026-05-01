import json
import logging
from pathlib import Path
from pydantic import BaseModel
from typing import List, Dict, Optional, Any
from src.backend.defaults import DEFAULT_SYSTEM_PROMPT


class LLMConfig(BaseModel):
    provider: str 
    model: str 
    base_url: str 
    api_key: str 
    max_tokens:int 
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
        """
        Initialize the proxy, load the system prompts from config.txt, 
        and spin up the Hugging Face model in memory.
        """
        self.bridge_url = bridge_url
        self.system_config = self._load_config(config_path)
        
        # Simulated Hugging Face initialization
        # self.tokenizer = AutoTokenizer.from_pretrained("...")
        # self.model = AutoModelForCausalLM.from_pretrained("...")

    def _load_json_file(self, path: Path, filename: str) -> dict:
        """
        Helper function to load the JSON files 
        """
        try:
            with open(path / filename, "r") as f:
                return json.load(f)
        #Error handling for json files
        except FileNotFoundError: 
            logging.error(f"{filename} not found. Please provide a valid config path.")
            raise
        except json.JSONDecodeError as e:
            logging.error(f"Invalid JSON in {filename}: {e}")
            raise 


    def _load_config(self, config_path):
        """
        Load each of the context JSON files and maps them to their respective dataclasses. 
        """ 
        path = Path(config_path)

        #Load the json files using the helper function 
        llm_config = self._load_json_file(path, "llm_config.json")
        json_schema = self._load_json_file(path, "json_schema.json")
        
        #Default system prompt file set to system_prompt.md
        system_prompt_path = path / "system_prompt.md"

        #Load system prompt
        if system_prompt_path.exists():
            logging.info(f"Loading system prompt from: {system_prompt_path}")
            try:
                with open(system_prompt_path, "r") as f:
                    system_prompt = f.read()
            #Fall back to hard coded default system prompt if errors occur
            except Exception as e:
                logging.warning(f"Failed to read system prompt file: {e}. Falling back to default.")
                system_prompt = DEFAULT_SYSTEM_PROMPT
        else:
            logging.warning(
                f"system_prompt.md not found at {system_prompt_path}. "
                f"Falling back to hardcoded default system prompt."
            )
            system_prompt = DEFAULT_SYSTEM_PROMPT

        return SystemConfig(
            llm=LLMConfig(**llm_config),
            json_schema=JSONSchema(**json_schema),
            system_prompt=system_prompt
        )



    def get_environment_context(self) -> list[str]:
        """
        Makes an HTTP GET request to the Bridge Node to fetch the Current Object List.
        Returns: ['blue_block', 'red_cube', 'table']
        """
        return ['blue_block', 'red_cube', 'table', 'window']




    def build_prompt(self, user_text: str, available_objects: List[str]) -> str:
        """
        Builds the final prompt sent to the LLM.
        It injects:
        - the system prompt (robot rules)
        - the recipe schema template
        - the list of valid target objects
        - the user's natural language command
        """

        schema_block = json.dumps(self.system_config.json_schema.model_dump(), indent=2)

        return (
            f"{self.system_config.system_prompt}\n\n"
            f"### Recipe Schema Template\n"
            f"{schema_block}\n\n"
            f"### Available Objects\n"
            f"{available_objects}\n\n"
            f"### User Command\n"
            f"{user_text}\n\n"
            f"### Task\n"
            f"Generate a valid JSON recipe following the schema above. "
            f"Only use allowed actions. Never output anything except JSON."
        )


    def generate_llm_response(self, formatted_prompt: str) -> str:
        """
        Passes the prompt to the Hugging Face model and returns the raw text generation.
        """
        # Dummy behavior based on prompt
        if "pick up" in formatted_prompt.lower() and "blue block" in formatted_prompt.lower():
            return '{"action": "pick_up", "target": "blue_block"}'
        elif "fly" in formatted_prompt.lower() and "window" in formatted_prompt.lower():
            return '{"action": "move_to", "target": "window"}'
        else:
            return '{"action": "unknown", "target": "unknown"}'



    def validate_and_extract_json(self, raw_llm_text: str) -> dict:
        """
        Finds the JSON block in the LLM's text output and checks it against your schema.
        Raises an Exception if the JSON is malformed.
        """
        try:
            parsed = json.loads(raw_llm_text)
            if "action" not in parsed or "target" not in parsed:
                raise ValueError("Missing 'action' or 'target' in JSON")
            return parsed
        except json.JSONDecodeError:
            raise ValueError("Invalid JSON format")

    def send_to_middleware(self, validated_json: dict) -> bool:
        """
        Makes an HTTP POST request to the Bridge Node.
        """
        # Dummy logic for success/failure
        if validated_json.get("action") == "move_to" and validated_json.get("target") == "window":
            return False # simulated kinematic failure
        return True

    def process_user_request(self, user_text: str) -> dict:
        """
        The main orchestrator function called by your User Interface.
        """
        try:
            available_objects = self.get_environment_context()
            prompt = self.build_prompt(user_text, available_objects)
            raw_response = self.generate_llm_response(prompt)
            validated_json = self.validate_and_extract_json(raw_response)
            success = self.send_to_middleware(validated_json)
            
            return {
                "prompt": prompt,
                "json": validated_json,
                "execution_result": "success" if success else "failure",
                "error": None if success else "Simulated kinematic failure. Target out of bounds."
            }
        except Exception as e:
            return {
                "prompt": f"User command: {user_text}",
                "json": None,
                "execution_result": "failure",
                "error": str(e)
            }
