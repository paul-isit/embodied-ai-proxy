import json
import logging
import uuid
import websocket
import requests
import threading
import re
from pathlib import Path
from pydantic import BaseModel
from typing import List, Dict, Optional, Any
from src.backend.defaults import DEFAULT_SYSTEM_PROMPT
from src.backend.llm_adapters import get_adapter
from src.backend.llm_config import LLMConfig

class StepSchema(BaseModel):
    step_id: int
    action: str
    description: Optional[str] = None
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
        self.ws = None
        self.ws_lock = threading.Lock()
        self.req_lock = threading.Lock()
        self.pending_requests = {}
        self.receive_thread = None
        self.on_connection_change = None
        self.adapter = get_adapter(self.llm_config)

    def connect(self):
        """Initializes a persistent WebSocket connection and dispatcher thread if one doesn't exist."""
        with self.ws_lock:
            if self.ws is None:
                self.ws = websocket.create_connection(self.bridge_url, timeout=60.0)

                if self.on_connection_change:
                    self.on_connection_change(True)
                    
                self.receive_thread = threading.Thread(target=self._receive_loop, daemon=True)
                self.receive_thread.start()

    def _receive_loop(self):
        """Background thread that listens for incoming WebSocket messages and routes them."""
        while True:
            try:
                result = self.ws.recv()
                response = json.loads(result)
                if response.get("op") == "service_response":
                    req_id = response.get("id")
                    with self.req_lock:
                        if req_id in self.pending_requests:
                            self.pending_requests[req_id]["response"] = response
                            self.pending_requests[req_id]["event"].set()
            except websocket.WebSocketTimeoutException:
                continue
                # Expected timeout due to inactivity, just keep listening
            except (
                websocket.WebSocketConnectionClosedException, ConnectionResetError, BrokenPipeError):
                logging.info("WebSocket connection closed by server.")
                break
            except Exception as e:
                logging.error(f"WebSocket receive thread error: {e}")
                break

        # Cleanup if we exit the loop
        with self.ws_lock:
            if self.ws:
                try:
                    self.ws.close()
                except Exception:
                    pass
            self.ws = None

        if self.on_connection_change:
            self.on_connection_change(False)

        # Wake up all pending requests with error
        with self.req_lock:
            for req in self.pending_requests.values():
                req["event"].set()

    def check_bridge_connection(self) -> bool:
        """
        Attempts to connect to the rosbridge WebSocket server.
        Returns True if successful, False otherwise.
        """
        try:
            self.connect()
            return True
        except (ConnectionRefusedError, websocket.WebSocketException) as e:
            logging.debug(f"Bridge connection failed: {e}")
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

        event = threading.Event()
        with self.req_lock:
            self.pending_requests[service_id] = {"event": event, "response": None}

        try:
            # Use persistent websocket connection
            self.connect()
            
            # Send the request
            with self.ws_lock:
                if self.ws is None:
                    raise Exception("WebSocket disconnected.")
                self.ws.send(json.dumps(call_msg))
            logging.info(f"Sent request to service: {service_name}")

            # Wait for response (with timeout to prevent infinite blocking)
            if not event.wait(timeout=60.0):
                raise Exception(
                    f"Timeout waiting for response from {service_name}")

            with self.req_lock:
                response = self.pending_requests[service_id]["response"]

            if response is None:
                raise Exception("WebSocket connection dropped while waiting for response.")

            if not response.get("result"):
                error_msg = (response.get("values") or "Service call failed (result: false)")
                raise Exception(f"ROS Service Error: {error_msg}")

            return response.get("values", {})

        except Exception as e:
            logging.error(f"Middleware connection failed: {e}")
            raise Exception(f"Middleware connection failed: {e}") from e
        finally:
            with self.req_lock:
                self.pending_requests.pop(service_id, None)

    def _load_json_file(self, path: Path, filename: str) -> dict:
        """
        Helper to load JSON files.
        """
        try:
            with open(path / filename, "r") as f:
                return json.load(f)
        #Error handling for json files
        except FileNotFoundError:
            logging.error(
                f"{filename} not found. Please provide a valid config path.")
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
        json_schema_raw = self._load_json_file(path, "json_schema.json")

        #Default system prompt file set to system_prompt.md
        system_prompt_path = path / "system_prompt.md"
 
        #Load system prompt
        if system_prompt_path.exists():
            logging.info(
                f"Loading system prompt from: {system_prompt_path}")
            try:
                with open(system_prompt_path, "r") as f:
                    system_prompt = f.read()
            #Fall back to hard coded default system prompt if errors occur
            except Exception as e:
                logging.warning(
                    f"Failed to read system prompt file: {e}. "
                    f"Falling back to default."
                )
                system_prompt = DEFAULT_SYSTEM_PROMPT

        else:
            logging.warning(
                f"system_prompt.md not found at {system_prompt_path}. "
                f"Falling back to hardcoded default system prompt."
            )
            system_prompt = DEFAULT_SYSTEM_PROMPT

        return SystemConfig(
            llm=LLMConfig(**llm_config),
            json_schema=JSONSchema(**json_schema_raw),
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
            raise RuntimeError(f"Failed to get environment context: {e}") from e

    def build_prompt(self, user_text: str,available_objects: list[str]) -> str:
        """
        Builds the final prompt by injecting dynamic variables into the 
        system_prompt template defined by the user.
        """
        # Convert schema to string
        schema_str = json.dumps(self.schema_block.model_dump(), indent=2)

        # Convert objects list to a nice string format
        objects_str = ("\n".join([f"- {obj}" for obj in available_objects]) if available_objects else "No objects currently mapped.")

        # Perform the template replacement using a dictionary to mirror the .format() structure
        # while safely ignoring actual JSON {} brackets in the markdown.
        try:
            replacements = {
                "{schema_template}": schema_str,
                "{available_objects}": objects_str,
                "{user_command}": user_text
            }

            final_prompt = self.system_prompt
            for key, value in replacements.items():
                if key not in final_prompt:
                    raise KeyError(key)
                final_prompt = final_prompt.replace(key, value)

            return final_prompt

        except KeyError as e:
        # Fallback in case the user deleted a required placeholder in the .md file     
            raise ValueError(f"system_prompt.md missing required placeholder: {e}")

    def generate_llm_response(self, formatted_prompt: str) -> str:
        """Passes the formatted prompt to the configured LLM adapter."""
        return self.adapter.generate(formatted_prompt)


    def validate_and_extract_json(self, raw_llm_text: str) -> dict:
        """
        Attempts to parse the JSON from the LLM.
        Raises ValueError if the output is not valid JSON.
        """
        # validation logic can be improved
        try:
            clean_text = raw_llm_text.strip()
            if clean_text.startswith("```json"):
                clean_text = clean_text[7:]
            elif clean_text.startswith("```"):
                clean_text = clean_text[3:]
                
            if clean_text.endswith("```"):
                clean_text = clean_text[:-3]
            clean_text = clean_text.strip()
            try:
                parsed_json_dict = json.loads(clean_text)
            except json.JSONDecodeError:

                match = re.search(r"\{.*\}", clean_text, re.DOTALL)

                if not match:
                    raise ValueError("No JSON object found in LLM response.")
                parsed_json_dict = json.loads(match.group(0))
            validated_data = JSONSchema(**parsed_json_dict).model_dump()
            return validated_data
        except Exception as e:
            raise ValueError(f"Failed to parse LLM output as JSON: {e}") from e

    def generate(
        self,
        user_prompt: str,
        available_objects: list[str]
    ):
        """
        Lightweight generation interface for
        automated YAML testing.
        Does NOT execute middleware.
        """

        prompt = self.build_prompt(
            user_prompt,
            available_objects
        )

        raw_response = self.generate_llm_response(prompt)

        validated_json = self.validate_and_extract_json(
            raw_response
        )

        return {
            "raw_output": raw_response,
            "parsed": validated_json
        }

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
            logging.error("Cannot connect to the robot. The rosbridge server is not running on port 9090.")
            return {
                "prompt": f"User command: {user_text}",
                "json": None,
                "execution_result": "failure",
                "error": "Cannot connect to the robot. The rosbridge server is not running on port 9090."
            }

        if not user_text.strip():
            return {"is_dummy": True}
            
        try:
            available_objects = self.get_environment_context()
            prompt = self.build_prompt(user_text, available_objects)
            raw_response = self.generate_llm_response(prompt)
            validated_json = self.validate_and_extract_json(raw_response)
            success = self.send_to_middleware(validated_json)

            return {
                "prompt": prompt,
                "json": validated_json,
                "execution_result": ("success" if success else "failure"
                ),
                "error": None if success else "Robot failed to execute the recipe."
            }
        except Exception as e:
            return {
                "prompt": f"User command: {user_text}",
                "json": None,
                "execution_result": "failure",
                "error": str(e)
            }
