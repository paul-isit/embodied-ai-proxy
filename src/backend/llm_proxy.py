import json
import logging
import sys
import uuid
import websocket
import requests
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
    def __init__(self, config_path: str, bridge_url: str = "ws://localhost:9090"):
        """
        Initialize the proxy, load the system prompts from config.txt, 
        and spin up the Hugging Face model in memory.
        """
        self.bridge_url = bridge_url
        self.system_config = self._load_config(config_path)
        self.llm_config = self.system_config.llm
        self.schema_block = self.system_config.json_schema
        self.system_prompt = self.system_config.system_prompt

    def check_bridge_connection(self) -> bool:
        """
        Attempts to connect to the rosbridge WebSocket server.
        Returns True if successful, False otherwise.
        """
        try:
            # Very short timeout just to check if the server is listening
            ws = websocket.create_connection(self.bridge_url, timeout=2.0)
            ws.close()
            return True
        except Exception:
            return False

    def _call_ros_service(self, service_name: str, args: dict = None) -> dict:
        """
        Connects to rosbridge, calls a ROS service synchronously, and returns the response.
        """
        args = args or {}
        service_id = f"call_service:{service_name}:{uuid.uuid4()}"
        
        call_msg = {
            "op": "call_service",
            "id": service_id,
            "service": service_name,
            "args": args
        }

        try:
            # Create a synchronous websocket connection
            ws = websocket.create_connection(self.bridge_url, timeout=60.0) # 60 second timeout for long robot movements
            
            # Send the request
            ws.send(json.dumps(call_msg))
            logging.info(f"Sent request to service: {service_name}")
            
            # Block and wait for the response
            while True:
                result = ws.recv()
                response = json.loads(result)
                
                # Check if this response matches our request ID
                if response.get("op") == "service_response" and response.get("id") == service_id:
                    ws.close()
                    
                    if not response.get("result"):
                        error_msg = response.get('values') or 'Service call failed (result: false)'
                        raise Exception(f"ROS Service Error: {error_msg}")
                    
                    return response.get("values", {})
                    
        except websocket.WebSocketTimeoutException:
            raise Exception(f"Timeout waiting for response from {service_name}")
        except Exception as e:
            logging.error(f"WebSocket error: {e}")
            raise Exception(f"Middleware connection failed: {e}")

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
        Makes a WebSocket request to rosbridge to fetch the Current Object List via a ROS Service.
        """
        try:
            # We assume the EnvironmentMappingNode provides a service /get_robot_parameters
            response = self._call_ros_service('/get_robot_parameters')
            return response.get('object_list', [])
        except Exception as e:
            logging.fatal(f"Failed to get environment context from middleware: {e}. Shutting down.")
            sys.exit(1)

    def build_prompt(self, user_text: str, available_objects: List[str]) -> str:
        """
        Builds the final prompt sent to the LLM.
        It injects:
        - the system prompt (robot rules)
        - the recipe schema template
        - the list of valid target objects
        - the user's natural language command
        """
        return (
            f"{self.system_prompt}\n\n"
            f"### Recipe Schema Template\nThis is the JSON Schema you should strictly follow when generating the JSON output. Understand that this is a template and you will have to modify and fill it based on the user command.\n"
            f"{json.dumps(self.schema_block.model_dump(), indent=2)}\n\n"
            f"### Available Objects\nThese are the list of available objects in the environment of the robot. ONLY use these objects while generating the JSON. If the user asks about an object which doesn't exist in this list, you need to respond with an error JSON. DO NO INVENT the objects if the user asks you to.\n"
            f"{available_objects}\n\n"
            f"### User Command\nThis is the user command, please generate the JSON for what the user is asking:\n"
            f"'{user_text}'\n\n"
        )


    def generate_llm_response(self, formatted_prompt: str) -> str:
        """Passes the formatted prompt to the remote LLM API (Ollama)."""
        payload = {
            "model": self.llm_config.model,
            "prompt": formatted_prompt,
            "stream": False,
            "options": {
                "temperature": self.llm_config.temperature
            }
        }

        print(f"payload: ", payload)
        try:
            # We add a timeout so the UI doesn't hang forever if the host isn't reachable
            response = requests.post(self.llm_config.base_url, json=payload, timeout=self.llm_config.timeout_seconds)
            if not response.ok:
                try:
                    error_msg = response.json().get("error", response.text)
                except ValueError:
                    error_msg = response.text
                raise RuntimeError(f"Ollama API Error ({response.status_code}): {error_msg}")
            
            data = response.json()
            return data.get("response", "").strip()
            
        except requests.exceptions.RequestException as e:
            raise RuntimeError(f"Failed to connect to Local LLM at {self.llm_config.base_url}. Error: {e}")



    def validate_and_extract_json(self, raw_llm_text: str) -> dict:
        """
        Dummy validation for testing end-to-end pipeline.
        Attempts to parse the JSON, but falls back to a dummy dict if it fails
        so the pipeline doesn't crash.
        """
        # this logic needs to be improved
        try:
            clean_text = raw_llm_text.strip()
            if clean_text.startswith("```json"):
                clean_text = clean_text[7:]
            elif clean_text.startswith("```"):
                clean_text = clean_text[3:]
                
            if clean_text.endswith("```"):
                clean_text = clean_text[:-3]
                
            clean_text = clean_text.strip()
            return json.loads(clean_text)
        except Exception as e:
            logging.warning(f"Dummy validation fallback used due to parse failure: {e}")
            return {"dummy_key": "dummy_value", "raw_response": raw_llm_text}

    def send_to_middleware(self, validated_json: dict) -> bool:
        """
        Makes a WebSocket request to rosbridge to execute the recipe via a ROS Service.
        """
        try:
            # We assume the JsonParserNode provides a service /execute_recipe
            response = self._call_ros_service('/execute_recipe', {"recipe_json": json.dumps(validated_json)})
            # We expect the service to return a success boolean
            return response.get('success', False)
        except Exception as e:
            logging.error(f"Failed to execute recipe: {e}")
            raise Exception(f"Failed to communicate with the robot: {str(e)}")

    def process_user_request(self, user_text: str) -> dict:
        """
        The main orchestrator function called by your User Interface.
        """
        if not self.check_bridge_connection():
            logging.fatal("Cannot connect to the robot. The rosbridge server is not running on port 9090. Shutting down.")
            sys.exit(1)

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
                "error": None if success else "Robot failed to execute the recipe."
            }
        except Exception as e:
            return {
                "prompt": f"User command: {user_text}",
                "json": None,
                "execution_result": "failure",
                "error": str(e)
            }
