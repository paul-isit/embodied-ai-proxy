from textual.widgets import Static


class StatusPanel(Static):
    """Shows bridge + system status."""

    def on_mount(self) -> None:
        self.connected = False
        self.refresh_view()

    def set_connection(self, state: bool) -> None:
        self.connected = state
        self.refresh_view()

    def refresh_view(self) -> None:
        state = "CONNECTED" if self.connected else "DISCONNECTED"
        self.update(f"Kinova Bridge: {state}")
