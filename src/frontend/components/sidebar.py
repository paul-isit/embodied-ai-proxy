from textual.app import ComposeResult
from textual.containers import Vertical
from textual.widgets import Static, Button

class SuggestedMovements(Vertical):
    def compose(self) -> ComposeResult:
        yield Static("--- MIDDLEWARE STATUS ---", classes="sidebar-title")
        yield Static("State: UNKNOWN", id="mw-overall-status", classes="sidebar-status-text")
        yield Static("", id="mw-nodes-status", classes="sidebar-status-text")
        
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

    def update_telemetry(self, msg: dict) -> None:
        overall = msg.get("summary_state", -1)
        nodes = msg.get("individual_states", [])
        
        # 0=READY, 1=BUSY, 2=FAULT
        state_map = {0: "READY", 1: "BUSY", 2: "FAULT"}
        overall_str = state_map.get(overall, "UNKNOWN")
        
        overall_widget = self.query_one("#mw-overall-status", Static)
        overall_widget.update(f"System: {overall_str}")
        
        node_lines = []
        for n in nodes:
            name = n.get("node_name", "unknown")
            state_val = n.get("state", 0)
            status_msg = n.get("status_message", "")
            
            n_state = state_map.get(state_val, "UNK")
            node_lines.append(f"• {name}: {n_state}\n  {status_msg}")
            
        nodes_widget = self.query_one("#mw-nodes-status", Static)
        nodes_widget.update("\n".join(node_lines))

