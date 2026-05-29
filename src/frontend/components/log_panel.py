import json
from textual.widgets import RichLog


class LogPanel(RichLog):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, markup=True, highlight=False, wrap=True, **kwargs)
        self.write("[bold #00aaff][SYS][/bold #00aaff] Session initialized. Awaiting commands...\n")

    def add_user(self, text: str) -> None:
        self.write(f"\n[bold #b284ff][USER][/bold #b284ff] {text}")

    def add_error(self, message: str) -> None:
        self.write(f"[bold #ff3333][ERR][/bold #ff3333] {message}")

    def add_info(self, message: str) -> None:
        self.write(f"[bold #00aaff][SYS][/bold #00aaff] {message}")

    def add_result(self, result: dict, latency: int, verbosity_level: int) -> None:
        execution_result = result.get("execution_result", "unknown")

        # Base (all verbosity levels)
        if "recipe" in result and result["recipe"]:
            self.write("[bold #ffffff][OK] Validated Robot Recipe:[/bold #ffffff]")
            clean_json = json.dumps(result["recipe"], indent=2)
            self.write(f"[#888888]{clean_json}[/#888888]")

            if execution_result == "success":
                self.write("[bold #00ff88]> Dispatched to middleware successfully.[/bold #00ff88]")
            else:
                self.write(f"[bold #ffaa00]> Middleware not confirmed ({execution_result}).[/bold #ffaa00]")
        else:
            self.write("[bold #ff3333][ERR] Schema parsing failure:[/bold #ff3333]")
            self.write("[#ff3333]LLM returned non-standard format. Ensure 'json-only' output mode.[/#ff3333]")

        # L2: built prompt
        if verbosity_level >= 2 and result.get("prompt"):
            self.write("\n[#888888]--- PROMPT ---[/#888888]")
            self.write(f"[#888888]{result['prompt']}[/#888888]")

        # L3: runtime metadata (no Size — raw output no longer exposed) 
        if verbosity_level >= 3:
            meta = result.get("meta", {})
            self.write("\n[#888888]--- METADATA ---[/#888888]")
            self.write(
                "[#888888]"
                f"Latency: {meta.get('latency_ms', latency)}ms | "
                f"Steps: {meta.get('step_count', 0)} | "
                f"CPU: {meta.get('cpu_hint', 'n/a')}"
                "[/#888888]"
            )

        self.write("\n[#888888]──────────────────────────────────────────────────[/#888888]\n")
