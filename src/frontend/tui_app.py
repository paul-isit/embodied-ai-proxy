import asyncio 
import time 
from pathlib import Path 

from textual.app import App 
from textual.binding import Binding 
from textual.widgets import Footer

from src.backend.llm_proxy import LLMProxy 
from src.frontend.components.status_panel import StatusPanel 
from src.frontend.components.log_panel import LogPanel 
from src.frontend.components.input_bar import InputBar 

BASE_DIR = Path(__file__).resolve().parent 

class EmbodiedProxyApp(App): 
    CSS_PATH = BASE_DIR / "styles.css" 
     
    show_chroma = False

    BINDINGS = [ 
        Binding("ctrl+y", "copy_prompt", "Ctrl+Y: Copy System Prompt", show=True), 
        Binding("ctrl+q", "quit", "Ctrl+Q: Quit Application", show=True) 
    ] 

    def __init__(self, proxy: LLMProxy, verbosity: int = 0): 
        super().__init__() 
        self.proxy = proxy 
        self.verbosity = verbosity 
        self.cached_system_prompt = None 

    def compose(self): 
        yield StatusPanel(id="status") 
        yield LogPanel(id="log") 
        yield InputBar(id="input") 
        yield Footer()

    def on_mount(self): 
        self.status = self.query_one("#status", StatusPanel) 
        self.log_panel = self.query_one("#log", LogPanel) 
        self.input = self.query_one("#input", InputBar) 
        self.title = "Embodied AI Proxy" 
        prompt_path = BASE_DIR.parent.parent / "configs" / "system_prompt.md" 
        if prompt_path.exists(): 
            try: 
                self.cached_system_prompt = prompt_path.read_text(encoding="utf-8").strip() 
            except Exception as e: 
                self.cached_system_prompt = f"Error reading system_prompt.md: {e}" 
        else: 
            self.cached_system_prompt = f"Warning: Configuration not found at {prompt_path}" 

        async def _bridge_loop(): 
            while True: 
                try: 
                    ok = self.proxy.check_bridge_connection() 
                    self.status.set_connection(ok) 
                except Exception: 
                    self.status.set_connection(False) 
                await asyncio.sleep(3) 

        asyncio.create_task(_bridge_loop()) 

    # HOTKEY ACTIONS
    def action_copy_prompt(self) -> None: 
        if self.cached_system_prompt: 
            self.copy_to_clipboard(self.cached_system_prompt) 
            self.notify("System Prompt copied to clipboard", title="Clipboard Action", severity="information") 
        else: 
            self.notify("No system prompt loaded to copy", severity="warning") 

    async def on_input_submitted(self, message): 
        text = message.value.strip() 
        if not text: 
            return 

        self.log_panel.add_user(text) 
        self.input.set_busy(True) 
        start = time.time() 

        try: 
            try: 
                current_objects = self.proxy.get_environment_context() 
            except Exception: 
                current_objects = [] 

            result = await asyncio.to_thread(self.proxy.generate, text, current_objects) 
            latency = int((time.time() - start) * 1000) 

            display_payload = {} 
            if "parsed" in result: 
                display_payload["recipe"] = result["parsed"] 
            if "raw_output" in result: 
                display_payload["raw"] = result["raw_output"] 

            display_payload["objects_mapped"] = current_objects 
            display_payload["model_name"] = getattr(self.proxy.llm_config, "model", "Ollama Default") 
            display_payload["endpoint"] = getattr(self.proxy.llm_config, "base_url", "N/A") 

            self.log_panel.add_result( 
                result=display_payload,  
                latency=latency,  
                verbosity_level=self.verbosity 
            ) 

        except Exception as e: 
            self.log_panel.add_error(str(e)) 
        finally: 
            self.input.set_busy(False)
