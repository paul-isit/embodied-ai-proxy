from textual.app import ComposeResult
from textual.containers import Vertical
from textual.widgets import Static, Button

class SuggestedMovements(Vertical):
    def compose(self) -> ComposeResult:
        yield Static("--- MOVEMENT EXAMPLES ---", classes="sidebar-title")
        yield Button("Pick up object", id="sug_pick", classes="suggested-btn")
        yield Button("Move Forward", id="sug_fwd", classes="suggested-btn")
        yield Button("Rotate Left", id="sug_rot", classes="suggested-btn")
        yield Button("Scan Arena", id="sug_scan", classes="suggested-btn")
        yield Button("Return Base", id="sug_base", classes="suggested-btn")
        yield Static(
            "CAPSTONE 2026\nROS2 MIDDLEWARE\nLLM PIPELINE\nVERSION 1.0.0", 
            classes="sidebar-footer"
        )
