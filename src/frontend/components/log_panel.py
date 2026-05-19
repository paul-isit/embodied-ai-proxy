import json
from textual.widgets import RichLog

class LogPanel(RichLog):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, markup=True, highlight=False, wrap=True, **kwargs)
        self.write("[bold #00aaff][SYS][/bold #00aaff] Session initialized. Awaiting commands...\n")

    def add_user(self, text: str) -> None:
        # Purple User tags
        self.write(f"\n[bold #b284ff][USER][/bold #b284ff] {text}")

    def add_error(self, message: str) -> None:
        self.write(f"[bold #ff3333][ERR][/bold #ff3333] {message}")

    def add_info(self, message: str) -> None:
        # Blue System tags
        self.write(f"[bold #00aaff][SYS][/bold #00aaff] {message}")

    def add_result(self, result: dict, latency: int, verbosity_level: int) -> None:
        if "recipe" in result and result["recipe"]:
            self.write("[bold #ffffff][OK] Validated Robot Recipe:[/bold #ffffff]")
            clean_json = json.dumps(result["recipe"], indent=2)
            self.write(f"[#888888]{clean_json}[/#888888]")
            
            action = result["recipe"].get("action", "")
            self.write(f"[#ffffff]> Execution sequence: '{action}' initialized.[/#ffffff]")
        else:
            # Re-formatted for a cleaner vintage look
            self.write("[bold #ff3333][ERR] Schema parsing failure:[/bold #ff3333]")
            self.write("[#ff3333]LLM returned non-standard format. Ensure 'json-only' output mode.[/#ff3333]")

        if verbosity_level >= 1 and "raw" in result and result["raw"]:
            self.write("\n[#888888]--- RAW OUTPUT ---[/#888888]")
            self.write(f"[#888888]{result['raw']}[/#888888]")

        if verbosity_level >= 2:
            objs = result.get("objects_mapped", [])
            obj_str = ", ".join(objs) if objs else "None detected"
            self.write("\n[#888888]--- CONTEXT ---[/#888888]")
            self.write(f"[#888888]Detected Objects: {obj_str}[/#888888]")

        if verbosity_level >= 3:
            model = result.get("model_name", "Unknown")
            endpoint = result.get("endpoint", "Unknown")
            self.write("\n[#888888]--- METADATA ---[/#888888]")
            self.write(f"[#888888]Model: {model} | Endpoint: {endpoint} | Latency: {latency}ms[/#888888]")

        self.write("\n[#888888]──────────────────────────────────────────────────[/#888888]\n")
