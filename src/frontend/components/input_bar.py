from textual.widgets import Input


class InputBar(Input):

    def on_mount(self):
        self.placeholder = "Enter command..."

    def set_busy(self, value: bool):
        """
        Disables input while LLM is processing.
        """
        self.disabled = value
        if value:
            self.placeholder = "Processing..."
        else:
            self.placeholder = "Enter command..."
