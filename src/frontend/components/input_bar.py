from textual.widgets import Input
from textual.events import Key

class InputBar(Input):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.placeholder = "Enter instruction... (Use Up/Down arrows for history)"

    def set_busy(self, busy: bool):
        self.disabled = busy
