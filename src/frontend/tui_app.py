import asyncio 
import time 
import logging
from pathlib import Path 

from textual import on
from textual.app import App 
from textual.containers import Horizontal, Vertical
from textual.widgets import Button, Static

from src.backend.llm_proxy import LLMProxy 
from src.frontend.components.status_panel import StatusPanel 
from src.frontend.components.log_panel import LogPanel 
from src.frontend.components.input_bar import InputBar
from src.frontend.components.sidebar import SuggestedMovements

# Supress raw terminal errors from ROS breaking the Textual UI, will likely remove after integration
logging.getLogger().setLevel(logging.CRITICAL + 1)

BASE_DIR = Path(__file__).resolve().parent 

class EmbodiedProxyApp(App): 
    CSS_PATH = BASE_DIR / "styles.css" 

    def __init__(self, proxy: LLMProxy, verbosity: int = 0): 
        super().__init__() 
        self.proxy = proxy 
        self.verbosity = verbosity 
        self.cached_system_prompt = None 

    def compose(self): 
        yield StatusPanel(id="status") 
        
        with Horizontal(id="toolbar"):
            yield Button("Cycle Mode", id="btn_mode")
            yield Button("System Info", id="btn_sys")
            yield Button("LLM Info", id="btn_llm")
            yield Button("Copy Prompt", id="btn_copy")
            yield Button("Quit", id="btn_quit")
            
        with Horizontal(id="main-body"):
            with Vertical(id="left-pane"):
                yield LogPanel(id="log") 
                yield Static("[ PROCESSING INFERENCE TARGET... ]", id="processing-bar") 
                yield InputBar(id="input") 
            yield SuggestedMovements(id="sidebar")

    def on_mount(self): 
        self.status = self.query_one("#status", StatusPanel) 
        self.log_panel = self.query_one("#log", LogPanel) 
        self.input = self.query_one("#input", InputBar) 
        self.processing_bar = self.query_one("#processing-bar", Static)

        self.title = "Embodied AI Proxy Workspace" 

        prompt_path = BASE_DIR.parent.parent / "configs" / "system_prompt.md" 
        if prompt_path.exists(): 
            try: 
                self.cached_system_prompt = prompt_path.read_text(encoding="utf-8").strip() 
            except Exception: 
                self.cached_system_prompt = "Error tracking system prompt." 

        async def _bridge_loop(): 
            while True: 
                try: 
                    ok = await asyncio.to_thread(self.proxy.check_bridge_connection)
                    self.status.set_connection(ok) 
                except Exception: 
                    self.status.set_connection(False) 
                await asyncio.sleep(5) 

        asyncio.create_task(_bridge_loop())

    @on(Button.Pressed, "#btn_mode")
    def handle_cycle_mode(self) -> None:
        self.verbosity = (self.verbosity + 1) % 4
        labels = ["0 (Minimal)", "1 (Filtered)", "2 (Full Map Context)", "3 (Engineering Debug)"]
        self.log_panel.add_info(f"Configuration modified: Verbosity is now Level {labels[self.verbosity]}")
        self.input.focus()

    @on(Button.Pressed, "#btn_sys")
    async def handle_sys_info(self) -> None:
        try:
            ok = await asyncio.to_thread(self.proxy.check_bridge_connection)
            status_markup = "[bold #00ff00]CONNECTED[/bold #00ff00]" if ok else "[bold #ff3333]DISCONNECTED[/bold #ff3333]"
            spacing = " " * (19 - len("CONNECTED") if ok else 19 - len("DISCONNECTED"))
            
            sys_box = (
                "\n┌────────────────────────────────────────┐\n"
                "│ SYSTEM OPERATIONAL FRAMEWORK MATRIX    │\n"
                "├────────────────────────────────────────┤\n"
                f"│ ROS Bridge Status : {status_markup}{spacing} │\n"
                "│ Pipeline Target   : /json_parser_node  │\n"
                "└────────────────────────────────────────┘"
            )
            self.log_panel.add_info(sys_box)
        except Exception as e:
            self.log_panel.add_error(f"System Matrix Fault: {str(e)}")
        self.input.focus()

    @on(Button.Pressed, "#btn_llm")
    def handle_llm_info(self) -> None:
        model = getattr(self.proxy.llm_config, "model", "Ollama Default")
        provider = getattr(self.proxy.llm_config, "provider", "Local Engine")
        
        llm_box = (
            "\n┌────────────────────────────────────────┐\n"
            "│ COGNITIVE INFERENCE MAPPING PROFILE    │\n"
            "├────────────────────────────────────────┤\n"
            f"│ Model Arch        : {model:<19} │\n"
            f"│ Token Provider    : {provider:<19} │\n"
            "└────────────────────────────────────────┘"
        )
        self.log_panel.add_info(llm_box)
        self.input.focus()

    @on(Button.Pressed, "#btn_copy")
    def handle_copy_prompt(self) -> None:
        if self.cached_system_prompt: 
            self.copy_to_clipboard(self.cached_system_prompt) 
            self.log_panel.add_info("System Prompt successfully copied to clipboard.")
        self.input.focus()

    @on(Button.Pressed, "#btn_quit")
    def handle_quit(self) -> None:
        self.exit()

    @on(Button.Pressed, ".suggested-btn")
    async def handle_suggested_movement(self, event: Button.Pressed) -> None:
        commands = {
            "sug_pick": "Pick up the nearest object",
            "sug_fwd": "Move 10 units forward",
            "sug_rot": "Rotate 90 degrees left",
            "sug_scan": "Scan environment",
            "sug_base": "Return to base"
        }
        cmd_text = commands.get(event.button.id, "Scan environment")
        await self.execute_transaction(cmd_text)

    @on(Button.Pressed, "#btn_mode")
    def handle_cycle_mode(self) -> None:
        self.verbosity = (self.verbosity % 3) + 1
        labels = {1: "1 (Filtered)", 2: "2 (Full Map Context)", 3: "3 (Engineering Debug)"}
        self.log_panel.add_info(f"Configuration modified: Verbosity is now Level {labels[self.verbosity]}")
        self.input.focus()

    async def on_input_submitted(self, message) -> None: 
        text = message.value.strip() 
        if not text: return 
        self.input.save_history(text)
        await self.execute_transaction(text)

    async def execute_transaction(self, text: str) -> None:
        self.log_panel.add_user(text) 
        self.input.value = ""
        self.input.set_busy(True) 
        self.processing_bar.add_class("visible")
        
        start = time.time() 
        
        try: 
            current_objects = await asyncio.to_thread(self.proxy.get_environment_context)
        except Exception as e: 
            current_objects = [] 
            if self.verbosity >= 2:
                self.log_panel.add_error(f"Failed to fetch environment context: {str(e)}")

        try:
            result = await asyncio.to_thread(self.proxy.generate, text, current_objects) 
            latency = int((time.time() - start) * 1000) 

            payload = {
                "recipe": result.get("parsed"),
                "raw": result.get("raw_output"),
                "objects_mapped": current_objects,
                "model_name": getattr(self.proxy.llm_config, "model", "Ollama Default"),
                "endpoint": getattr(self.proxy.llm_config, "base_url", "N/A")
            }

            self.log_panel.add_result(payload, latency, self.verbosity) 

        except Exception as e: 
            self.log_panel.add_error(str(e)) 
        finally: 
            self.processing_bar.remove_class("visible")
            self.input.set_busy(False) 
            self.input.focus()
