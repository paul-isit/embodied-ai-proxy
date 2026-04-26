import json

class LLMProxy:
    def __init__(self, bridge_url: str = "http://localhost:9090", config_path: str = ""):
        """
        Initialize the proxy, load the system prompts from config.txt, 
        and spin up the Hugging Face model in memory.
        """
        self.bridge_url = bridge_url
        # self.system_prompt_template = self._load_config(config_path)
        
        # Simulated Hugging Face initialization
        # self.tokenizer = AutoTokenizer.from_pretrained("...")
        # self.model = AutoModelForCausalLM.from_pretrained("...")

    def _load_config(self, filepath: str) -> str:
        """Reads config.txt to get the base instructions and JSON schema rules."""
        return "Dummy System Prompt"

    def get_environment_context(self) -> list[str]:
        """
        Makes an HTTP GET request to the Bridge Node to fetch the Current Object List.
        Returns: ['blue_block', 'red_cube', 'table']
        """
        return ['blue_block', 'red_cube', 'table', 'window']

    def build_prompt(self, user_text: str, available_objects: list[str]) -> str:
        """
        Injects the available objects and the user's command into the system prompt.
        """
        return f"Valid targets are {available_objects}. User command: {user_text}"

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
