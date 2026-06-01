from textual.widgets import Static

class StatusPanel(Static):
    def set_connection(self, is_connected: bool) -> None:
        if is_connected:
            self.update("Embodied AI Proxy | Bridge: [bold #00ff00]CONNECTED[/bold #00ff00]")
        else:
            self.update("Embodied AI Proxy | Bridge: [bold #ff3333]DISCONNECTED[/bold #ff3333]")
