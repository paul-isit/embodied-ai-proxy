import asyncio
import time
import logging
import os
from pathlib import Path
from typing import Optional

from textual import on
from textual.app import App
from textual.containers import Horizontal, Vertical
from textual.events import Key
from textual.widgets import Button, Static

from src.backend.llm_proxy import LLMProxy
from src.frontend.components.status_panel import StatusPanel
from src.frontend.components.log_panel import LogPanel
from src.frontend.components.input_bar import InputBar
from src.frontend.components.sidebar import SuggestedMovements


logging.getLogger().setLevel(logging.CRITICAL + 1)

BASE_DIR = Path(__file__).resolve().parent


class EmbodiedProxyApp(App):
    CSS_PATH = BASE_DIR / "styles.css"

    def __init__(self, proxy: LLMProxy, verbosity: int = 0):
        super().__init__()
        self.proxy = proxy
        self.verbosity = verbosity
        # UI components (populated on_mount)
        self.status: Optional[StatusPanel] = None
        self.log_panel: Optional[LogPanel] = None
        self.input: Optional[InputBar] = None
        self.processing_bar: Optional[Static] = None
        self.sidebar: Optional[SuggestedMovements] = None
        # state
        self._busy = False
        self._cooldown_until = 0.0
        self.bridge_connected = False
        self.checking_bridge = False
        # history
        self._history: list[str] = []
        self._history_index: int = -1
        self._history_draft: str = ""

    # UI Layout
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

    # Init Hook Mounting
    def on_mount(self):
        self.status = self.query_one("#status", StatusPanel)
        self.log_panel = self.query_one("#log", LogPanel)
        self.input = self.query_one("#input", InputBar)
        self.processing_bar = self.query_one("#processing-bar", Static)
        self.sidebar = self.query_one("#sidebar", SuggestedMovements)
        self.title = "Embodied AI Proxy Workspace"
        # single authoritative bridge polling loop
        self.set_interval(5.0, self._check_bridge_status)

    # Guard TUI State
    def _ui_blocked(self) -> bool:
        return self._busy or (time.time() < self._cooldown_until)

    # Bridge Status
    async def _check_bridge_status(self) -> None:
        if self.checking_bridge:
            return
            
        if not self.app:
            return

        self.checking_bridge = True
        try:
            ok = await asyncio.to_thread(self.proxy.check_bridge_connection)
            self.bridge_connected = ok

            if self.status:
                self.status.set_connection(ok)

        except Exception:
            self.bridge_connected = False
            if self.status:
                self.status.set_connection(False)

        finally:
            self.checking_bridge = False

    # Lock the UI
    def _set_busy(self, busy: bool) -> None:
        self._busy = busy

        if self.input:
            self.input.disabled = busy

        if self.sidebar:
            self.sidebar.disabled = busy
            self.sidebar.can_focus = not busy

        for button in self.query(Button):
            button.disabled = busy

        if self.processing_bar:
            if busy:
                self.processing_bar.add_class("visible")
            else:
                self.processing_bar.remove_class("visible")

    # Block any events
    def on_button_pressed(self, event: Button.Pressed) -> None:
        if self._ui_blocked():
            event.prevent_default()
            event.stop()

    def on_key(self, event: Key) -> None:
        if not self.input or not self.input.has_focus:
            return

        if not self._history:
            return

        if event.key == "up":
            if self._history_index == -1:
                self._history_draft = self.input.value

            if self._history_index + 1 < len(self._history):
                self._history_index += 1
                self.input.value = self._history[self._history_index]
                self.input.cursor_position = len(self.input.value)

            event.stop()

        elif event.key == "down":
            if self._history_index == -1:
                return

            self._history_index -= 1

            if self._history_index < 0:
                self._history_index = -1
                self.input.value = self._history_draft
            else:
                self.input.value = self._history[self._history_index]

            self.input.cursor_position = len(self.input.value)
            event.stop()

    # Toolbar Handlers
    @on(Button.Pressed, "#btn_mode")
    def handle_cycle_mode(self) -> None:
        self.verbosity = (self.verbosity % 3) + 1
        labels = {1: "L1 - Filtered", 2: "L2 - Full Context", 3: "L3 - Debug"}

        if self.log_panel:
            self.log_panel.add_info(f"Verbosity set to {labels[self.verbosity]}")

        if self.input:
            self.input.focus()

    @on(Button.Pressed, "#btn_sys")
    def handle_sys_info(self) -> None:
        status = "[CONNECTED]" if self.bridge_connected else "[DISCONNECTED]"
        state = "LOCKED" if self._ui_blocked() else "READY"

        v_labels = {1: "L1 - Filtered", 2: "L2 - Full Context", 3: "L3 - Debug"}

        if self.log_panel:
            self.log_panel.add_info(
                "SYSTEM OPERATIONAL STATUS\n"
                f"Bridge Node : {status}\n"
                f"Target Node : /json_parser_node\n"
                f"UI State    : {state}\n"
                f"Verbosity   : {v_labels.get(self.verbosity)}\n"
            )

        if self.input:
            self.input.focus()

    @on(Button.Pressed, "#btn_llm")
    def handle_llm_info(self) -> None:
        sys_cfg = getattr(self.proxy, "system_config", None)
        llm = getattr(sys_cfg, "llm", None) if sys_cfg else getattr(self.proxy, "llm_config", None)

        model = getattr(llm, "model", "UNKNOWN")
        provider = getattr(llm, "provider", "UNKNOWN")
        endpoint = getattr(llm, "base_url", "UNKNOWN")
        temp = getattr(llm, "temperature", "UNKNOWN")

        if self.log_panel:
            self.log_panel.add_info(
                "LLM INFERENCE CONFIGURATION\n"
                f"Model       : {model}\n"
                f"Provider    : {provider}\n"
                f"Endpoint    : {endpoint}\n"
                f"Temperature : {temp}\n"
            )

        if self.input:
            self.input.focus()

    @on(Button.Pressed, "#btn_copy")
    def handle_copy_prompt(self) -> None:
        try:
            system_prompt = self.proxy.system_prompt
        except Exception:
            system_prompt = None

        if system_prompt:
            self.copy_to_clipboard(system_prompt)
            self.log_panel.add_info("System prompt copied.")
        else:
            self.log_panel.add_error("System prompt unavailable.")

        if self.input:
            self.input.focus()

    @on(Button.Pressed, "#btn_quit")
    def handle_quit(self) -> None:
        self.exit()

    # Side Button Commands
    @on(Button.Pressed)
    async def handle_any_button(self, event: Button.Pressed) -> None:
        if self._ui_blocked():
            return

        button_id = event.button.id or ""

        commands = {
            "sug_pick": "Pick up the nearest object",
            "sug_fwd": "Move 10 units forward",
            "sug_rot": "Rotate 90 degrees left",
            "sug_scan": "Scan environment",
            "sug_base": "Return to base",
        }

        cmd = commands.get(button_id)
        if cmd is None:
            return

        await self.execute_transaction(cmd)

    # Input Handling
    async def on_input_submitted(self, message) -> None:
        if self._ui_blocked():
            return

        text = message.value.strip()
        if not text:
            return

        await self.execute_transaction(text)

    # Core Execution Pipeline
    async def execute_transaction(self, text: str) -> None:
        if self._ui_blocked():
            return

        self._set_busy(True)
        self._history_index = -1
        self._history_draft = ""

        if self.log_panel:
            self.log_panel.add_info("Sending request to LLM proxy...")
            self.log_panel.add_user(text)

        if self.input:
            self.input.value = ""

        start = time.time()

        try:
            result = await asyncio.to_thread(self.proxy.process_user_request, text)
            latency = int((time.time() - start) * 1000)

            # process_user_request returns {"is_dummy": True} for empty input —
            # the guard in on_input_submitted should prevent this, but handle it
            # defensively so sidebar quick-commands can't slip through either.
            if result.get("is_dummy"):
                return

            parsed = result.get("json") or {}
            prompt = result.get("prompt", "")
            execution_result = result.get("execution_result", "unknown")
            error = result.get("error")

            # Surface any pipeline error (bridge down, LLM failure, bad JSON, etc.)
            # returned by process_user_request without raising.
            if error:
                if self.log_panel:
                    self.log_panel.add_error(f"Pipeline error: {error}")
            else:
                meta = {
                    "latency_ms": latency,
                    "step_count": len(parsed.get("steps", [])),
                    "execution_result": execution_result,
                    "cpu_hint": os.getloadavg()[0] if hasattr(os, "getloadavg") else "n/a",
                }

                # L1 — recipe only
                # L2 — recipe + built prompt (replaces raw LLM text; prompt is
                #       more useful for debugging than the unvalidated LLM output,
                #       and process_user_request does not expose raw LLM text)
                # L3 — recipe + prompt + runtime meta
                if self.verbosity == 1:
                    payload = {
                        "recipe": parsed,
                        "execution_result": execution_result,
                    }
                elif self.verbosity == 2:
                    payload = {
                        "recipe": parsed,
                        "execution_result": execution_result,
                        "prompt": prompt,
                    }
                else:
                    payload = {
                        "recipe": parsed,
                        "execution_result": execution_result,
                        "prompt": prompt,
                        "meta": meta,
                    }

                if self.log_panel:
                    self.log_panel.add_info("Response received")
                    self.log_panel.add_result(payload, latency, self.verbosity)

            # Record to history regardless of pipeline error — the user typed a
            # real command and should be able to navigate back to it.
            self._history.insert(0, text)

        except Exception as e:
            # Only truly unexpected exceptions (e.g. asyncio failure, internal
            # proxy crash) land here; normal pipeline errors come back in the
            # result dict above.
            if self.log_panel:
                self.log_panel.add_error(f"Unexpected error: {e}")

        finally:
            await asyncio.sleep(0.1)

            self._cooldown_until = time.time() + 0.5
            self._set_busy(False)

            if self.input:
                self.input.focus()
