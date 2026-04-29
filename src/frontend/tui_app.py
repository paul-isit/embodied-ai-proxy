from textual import work
from textual.app import App, ComposeResult
from textual.widgets import Header, Footer, Input, RichLog
from src.backend.llm_proxy import LLMProxy

class EmbodiedProxyApp(App):
    """A Textual app for the Embodied AI Proxy.""" 

    CSS = """
    Screen {
        layout: vertical;
    }
    
    #log-view {
        height: 1fr;
        border: solid green;
        padding: 1;
        margin: 1;
    }

    #input-bar {
        dock: bottom;
        margin: 1;
    }
    """

    def __init__(self, proxy: LLMProxy):
        super().__init__()
        self.proxy = proxy

    def compose(self) -> ComposeResult:
        """Create child widgets for the app."""
        yield Header()
        yield RichLog(id="log-view", highlight=True, markup=True)
        yield Input(placeholder="Enter instruction for the robot...", id="input-bar")
        yield Footer()

    def on_mount(self) -> None:
        """Set up the app after mounting."""
        self.title = "Embodied AI Proxy"
        self.sub_title = "Bridge: CONNECTED (Simulated)"
        self.query_one(Input).focus()

    async def on_input_submitted(self, event: Input.Submitted) -> None:
        """Handle when the user hits enter in the input bar."""
        user_text = event.value
        if not user_text.strip():
            return
            
        event.input.value = ""
        
        # Show a loading state in the log while processing
        log_view = self.query_one(RichLog)
        log_view.write(f"Prompt: > '{user_text}'\n[dim]Processing...[/dim]")
        
        # Fire and forget the background worker
        self.fetch_proxy_response(user_text)

    @work(exclusive=True, thread=True)
    def fetch_proxy_response(self, user_text: str) -> None:
        """This runs in a background thread and doesn't block the UI."""
        result = self.proxy.process_user_request(user_text)
        
        # Once done, update the UI using Textual's thread-safe message passing
        self.call_from_thread(self.update_log_view, result)

    def update_log_view(self, result: dict) -> None:
        """Called from the background thread to safely update the UI."""
        # Format the output using Textual's rich markup instead of hardcoded emojis
        log_text = ""
        
        if result["json"]:
            log_text += f"Validated JSON: {result['json']}\n"
        else:
            log_text += f"Validated JSON: [NONE]\n"
            
        if result["execution_result"] == "success":
            log_text += "[bold green][PASS] SUCCESS: Action completed successfully.[/bold green]\n---"
        else:
            reason = result.get("error", "Unknown error")
            log_text += f"[bold red][FAIL] FAILURE: {reason}[/bold red]\n---"
            
        # Write to the RichLog
        log_view = self.query_one(RichLog)
        log_view.write(log_text)
