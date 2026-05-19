from textual.widgets import Input
from textual.events import Key

class InputBar(Input):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.history = []
        self.history_index = -1
        self.placeholder = "Enter instruction... (Use Up/Down arrows for history)"

    def set_busy(self, busy: bool):
        self.disabled = busy

    def save_history(self, text: str):
        if text and (not self.history or self.history[-1] != text):
            self.history.append(text)
        self.history_index = len(self.history)

    async def on_key(self, event: Key):
        if event.key == "up":
            if self.history and self.history_index > 0:
                self.history_index -= 1
                self.value = self.history[self.history_index]
            event.prevent_default()
        elif event.key == "down":
            if self.history and self.history_index < len(self.history) - 1:
                self.history_index += 1
                self.value = self.history[self.history_index]
            elif self.history_index == len(self.history) - 1:
                self.history_index += 1
                self.value = ""
            event.prevent_default()
