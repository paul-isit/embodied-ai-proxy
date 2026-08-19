#!/usr/bin/env python3
"""
bridge_client.py

WebSocket client node that bridges the Go backend and ROS2's rosbridge_server.

Two independent WebSocket connections are maintained:

1. Backend-facing (NEW): a client connection to the Go backend's
   ``ws://localhost:8080/ws/bridge`` endpoint. Messages are JSON envelopes
   of the form ``{"type": "...", "payload": {...}}`` matching the protocol
   implemented by the Go backend's WebSocket hub
   (``backend/internal/websocket/hub.go``).

2. Rosbridge-facing (LIFTED from the legacy ``src/backend/llm_proxy.py``
   ``LLMProxy`` class): a client connection to the still-unchanged
   ``rosbridge_websocket`` node started by ``proxy_bridge.launch.py`` on
   ``ws://localhost:9090``, used to invoke the Kinova middleware's
   ``/get_robot_parameters`` and ``/execute_recipe`` ROS services.

This node's job is purely to translate between the two: on start (and then
periodically, so the backend's prompt pipeline never works from a stale
list) it asks rosbridge for the current workspace object list and reports it
to the Go backend as a ``status_update``; when the Go backend broadcasts a
successful ``action_recipe``, it forwards the recipe to the Kinova
middleware via ``/execute_recipe`` (on its own thread, so a slow/blocked
recipe never stalls the backend WebSocket receive loop) and reports the
outcome back to the Go backend as a ``status_update`` with an
``execution_result`` field. It also subscribes to the middleware's
``/system/status`` topic and relays per-node telemetry to the backend as a
``status_update`` with a ``middleware_status`` field.
"""
import argparse
import json
import logging
import os
import threading
import time
import uuid

import websocket


logger = logging.getLogger("bridge_client")

# Envelope "type" values, matching backend/internal/websocket/hub.go
TYPE_PROMPT_SUBMIT = "prompt_submit"
TYPE_ACTION_RECIPE = "action_recipe"
TYPE_STATUS_UPDATE = "status_update"
TYPE_LOG_EVENT = "log_event"

DEFAULT_BACKEND_WS_URL = "ws://localhost:8080/ws/bridge"
DEFAULT_ROSBRIDGE_WS_URL = "ws://localhost:9090"
DEFAULT_RECONNECT_DELAY = 5.0
DEFAULT_OBJECT_LIST_REFRESH_INTERVAL = 10.0


class RosbridgeConnection:
    """
    Thin client for the rosbridge_server WebSocket protocol
    (``ws://localhost:9090`` by default).

    The connect / receive-loop / service-call logic here is lifted, with
    minimal changes (no LLM/prompt concerns, no telemetry subscription),
    from the legacy ``LLMProxy`` class in ``src/backend/llm_proxy.py``,
    which is the reference implementation for talking to rosbridge_server
    and the Kinova middleware's services.
    """

    def __init__(self, rosbridge_url: str):
        self.rosbridge_url = rosbridge_url
        self.ws = None
        self.ws_lock = threading.Lock()
        self.req_lock = threading.Lock()
        self.pending_requests = {}
        self.receive_thread = None
        # Set by BackendBridgeClient to receive /system/status telemetry
        # messages as they arrive, mirroring the legacy LLMProxy.on_telemetry_update.
        self.on_telemetry_update = None

    def connect(self):
        """Initializes a persistent WebSocket connection and dispatcher thread if one doesn't exist."""
        with self.ws_lock:
            if self.ws is None:
                self.ws = websocket.create_connection(self.rosbridge_url, timeout=60.0)
                subscribe_msg = {
                    "op": "subscribe",
                    "topic": "/system/status",
                    "type": "kinova_interfaces/msg/SystemSummary",
                }
                self.ws.send(json.dumps(subscribe_msg))
                self.receive_thread = threading.Thread(target=self._receive_loop, daemon=True)
                self.receive_thread.start()

    def _receive_loop(self):
        """Background thread that listens for incoming WebSocket messages and routes them."""
        while True:
            try:
                result = self.ws.recv()
                response = json.loads(result)
                if response.get("op") == "service_response":
                    req_id = response.get("id")
                    with self.req_lock:
                        if req_id in self.pending_requests:
                            self.pending_requests[req_id]["response"] = response
                            self.pending_requests[req_id]["event"].set()
                elif response.get("op") == "publish" and response.get("topic") == "/system/status":
                    if self.on_telemetry_update:
                        self.on_telemetry_update(response.get("msg", {}))
            except websocket.WebSocketTimeoutException:
                # Expected timeout due to inactivity, just keep listening
                continue
            except (
                websocket.WebSocketConnectionClosedException, ConnectionResetError, BrokenPipeError):
                logger.info("Rosbridge WebSocket connection closed by server.")
                break
            except Exception as e:
                logger.error(f"Rosbridge receive thread error: {e}")
                break

        # Cleanup if we exit the loop
        with self.ws_lock:
            if self.ws:
                try:
                    self.ws.close()
                except Exception:
                    pass
            self.ws = None

        # Wake up all pending requests with error
        with self.req_lock:
            for req in self.pending_requests.values():
                req["event"].set()

    def check_connection(self) -> bool:
        """
        Attempts to connect to the rosbridge WebSocket server.
        Returns True if successful, False otherwise.
        """
        try:
            self.connect()
            return True
        except (ConnectionRefusedError, websocket.WebSocketException, OSError) as e:
            logger.debug(f"Rosbridge connection failed: {e}")
            return False

    def call_service(self, service_name: str, args: dict = None) -> dict:
        """
        Connects to rosbridge, calls a ROS service synchronously, and returns the response.
        """
        args = args or {}
        service_id = f"call_service:{service_name}:{uuid.uuid4()}"

        call_msg = {
            "op": "call_service",
            "id": service_id,
            "service": service_name,
            "args": args,
        }

        event = threading.Event()
        with self.req_lock:
            self.pending_requests[service_id] = {"event": event, "response": None}

        try:
            # Use persistent websocket connection
            self.connect()

            with self.ws_lock:
                if self.ws is None:
                    raise Exception("WebSocket disconnected.")
                self.ws.send(json.dumps(call_msg))
            logger.info(f"Sent request to service: {service_name}")

            # Wait for response (with timeout to prevent infinite blocking)
            if not event.wait(timeout=60.0):
                raise Exception(f"Timeout waiting for response from {service_name}")

            with self.req_lock:
                response = self.pending_requests[service_id]["response"]

            if response is None:
                raise Exception("WebSocket connection dropped while waiting for response.")

            if not response.get("result"):
                error_msg = response.get("values") or "Service call failed (result: false)"
                raise Exception(f"ROS Service Error: {error_msg}")

            return response.get("values", {})

        except Exception as e:
            logger.error(f"Middleware connection failed: {e}")
            raise Exception(f"Middleware connection failed: {e}") from e
        finally:
            with self.req_lock:
                self.pending_requests.pop(service_id, None)

    def get_environment_context(self) -> list:
        """
        Makes a WebSocket request to rosbridge to fetch the current object list
        via the ``/get_robot_parameters`` ROS service.
        """
        try:
            response = self.call_service("/get_robot_parameters")
            return response.get("object_list", [])
        except Exception as e:
            error_str = str(e)
            if "does not exist" in error_str:
                logger.warning(
                    "ROS service /get_robot_parameters not found "
                    "(EnvironmentMappingNode may not be running). "
                    "Proceeding with empty object list."
                )
                return []
            # Any other failure (timeout, disconnected, etc.) is a genuine
            # transport error.
            raise RuntimeError(f"Failed to get environment context: {e}") from e

    def execute_recipe(self, recipe_json: dict) -> bool:
        """
        Makes a WebSocket request to rosbridge to execute a validated action
        recipe via the ``/execute_recipe`` ROS service.
        """
        try:
            # We assume the JsonParserNode provides a service /execute_recipe
            response = self.call_service(
                "/execute_recipe", {"recipe_json": json.dumps(recipe_json)}
            )
            # We expect the service to return a success boolean
            return response.get("success", False)
        except Exception as e:
            error_str = str(e)
            if "does not exist" in error_str:
                logger.error("ROS service /execute_recipe not found (JsonParserNode may not be running).")
                raise Exception(
                    "ROS service /execute_recipe not found (JsonParserNode may not be running)."
                )
            # Any other failure is a genuine transport error.
            logger.error(f"Failed to execute recipe: {e}")
            raise Exception(f"Failed to communicate with the robot: {str(e)}")


class BackendBridgeClient:
    """
    WebSocket client connecting to the Go backend's ``/ws/bridge`` endpoint.

    Translates the Go backend's JSON envelope protocol
    (``{"type": ..., "payload": ...}``) into ROS2 service calls against
    rosbridge_server via a ``RosbridgeConnection``, and reports execution
    status back to the backend.
    """

    def __init__(
        self,
        backend_url: str,
        rosbridge: RosbridgeConnection,
        reconnect_delay: float = DEFAULT_RECONNECT_DELAY,
        object_list_refresh_interval: float = DEFAULT_OBJECT_LIST_REFRESH_INTERVAL,
    ):
        self.backend_url = backend_url
        self.rosbridge = rosbridge
        self.reconnect_delay = reconnect_delay
        self.object_list_refresh_interval = object_list_refresh_interval
        self.ws = None
        self.send_lock = threading.Lock()
        self._stop = threading.Event()
        self.rosbridge.on_telemetry_update = self._publish_telemetry

    def _send(self, msg_type: str, payload: dict) -> None:
        """Serializes and sends a single envelope to the Go backend, if connected."""
        envelope = {"type": msg_type, "payload": payload}
        with self.send_lock:
            if self.ws is None:
                logger.warning(f"Cannot send '{msg_type}': not connected to Go backend.")
                return
            try:
                self.ws.send(json.dumps(envelope))
            except Exception as e:
                logger.error(f"Failed to send '{msg_type}' to Go backend: {e}")

    def _publish_object_list(self) -> None:
        """
        Fetches the current workspace object list from ROS (via rosbridge)
        and reports it to the Go backend. This is how the backend's prompt
        pipeline learns what objects exist in the workspace, and doubles as
        the bridge's "registration"/availability signal since the protocol
        has no dedicated register message type.

        Note: the Go backend's bridge WebSocket handler
        (``Hub.ServeBridge`` in ``backend/internal/websocket/hub.go``) only
        accepts ``status_update`` envelopes from the bridge connection - any
        other type is logged and ignored - so this bridge only ever sends
        ``status_update`` messages, never ``log_event`` (that type is for
        the backend's own broadcasts to TUI/eval clients).
        """
        try:
            object_list = self.rosbridge.get_environment_context()
        except Exception as e:
            logger.error(f"Failed to fetch environment context: {e}")
            object_list = []

        logger.info(f"Reporting {len(object_list)} workspace object(s) to Go backend.")
        self._send(TYPE_STATUS_UPDATE, {"object_list": object_list})

    def _publish_telemetry(self, msg: dict) -> None:
        """
        Called from the rosbridge receive thread whenever a ``/system/status``
        message arrives; relays it to the Go backend verbatim under a
        ``middleware_status`` field (per-node READY/BUSY/FAULT state).
        """
        self._send(TYPE_STATUS_UPDATE, {"middleware_status": msg})

    def _handle_action_recipe(self, payload) -> None:
        """
        Dispatches a validated, successful action recipe to the Kinova
        middleware via ``/execute_recipe`` and reports the outcome back to
        the Go backend as a ``status_update``. Error-status recipes should
        not normally reach the bridge (the backend routes those as
        ``log_event`` instead), but we defensively ignore them here too.
        """
        if not isinstance(payload, dict) or payload.get("status") != "success":
            logger.debug(f"Ignoring non-success action_recipe payload: {payload}")
            return

        recipe_name = payload.get("recipe_name", "<unnamed recipe>")
        logger.info(f"Dispatching action recipe '{recipe_name}' to middleware.")

        try:
            success = self.rosbridge.execute_recipe(payload)
            execution_result = "success" if success else "failure"
            error_msg = None if success else "Robot failed to execute the recipe."
            logger.info(f"Recipe '{recipe_name}' execution result: {execution_result}")
        except Exception as e:
            execution_result = "failure"
            error_msg = str(e)
            logger.error(f"Recipe '{recipe_name}' execution failed: {error_msg}")

        self._send(
            TYPE_STATUS_UPDATE,
            {"execution_result": execution_result, "error": error_msg},
        )

    def _receive_loop(self) -> None:
        """Listens for envelopes from the Go backend until disconnected or stopped."""
        while not self._stop.is_set():
            try:
                raw = self.ws.recv()
            except websocket.WebSocketTimeoutException:
                continue
            except (
                websocket.WebSocketConnectionClosedException, ConnectionResetError, BrokenPipeError):
                logger.info("Go backend WebSocket connection closed.")
                break
            except Exception as e:
                logger.error(f"Go backend receive loop error: {e}")
                break

            if raw == "":
                # websocket-client can return an empty string (rather than
                # raising) when the peer closes the connection.
                logger.info("Go backend WebSocket connection closed.")
                break

            try:
                envelope = json.loads(raw)
            except json.JSONDecodeError:
                logger.warning(f"Received malformed envelope from Go backend: {raw!r}")
                continue

            msg_type = envelope.get("type")
            payload = envelope.get("payload") or {}

            if msg_type == TYPE_ACTION_RECIPE:
                # Execute on its own thread: execute_recipe can block for up
                # to 60s waiting on the middleware, and this loop must keep
                # reading subsequent envelopes from the Go backend meanwhile.
                threading.Thread(
                    target=self._handle_action_recipe, args=(payload,), daemon=True
                ).start()
            else:
                # prompt_submit / log_event / unknown types aren't consumed
                # by this bridge; only action_recipe requires a reaction.
                logger.debug(f"Ignoring envelope of type '{msg_type}' from Go backend.")

        with self.send_lock:
            if self.ws:
                try:
                    self.ws.close()
                except Exception:
                    pass
            self.ws = None

    def _object_list_refresh_loop(self, cycle_stop: threading.Event) -> None:
        """
        Background thread started for the lifetime of one backend connection:
        periodically re-publishes the workspace object list so the backend's
        prompt pipeline never works from a list that's gone stale mid-session.
        Stops as soon as ``cycle_stop`` is set (see run_forever).
        """
        while not cycle_stop.wait(self.object_list_refresh_interval):
            self._publish_object_list()

    def run_forever(self) -> None:
        """
        Connects to the Go backend and processes messages until ``stop()``
        is called, automatically reconnecting (with a fixed delay) if the
        connection drops or cannot be established.
        """
        while not self._stop.is_set():
            cycle_stop = threading.Event()
            try:
                logger.info(f"Connecting to Go backend at {self.backend_url} ...")
                with self.send_lock:
                    self.ws = websocket.create_connection(self.backend_url, timeout=60.0)
                logger.info("Connected to Go backend.")
                self._publish_object_list()
                threading.Thread(
                    target=self._object_list_refresh_loop, args=(cycle_stop,), daemon=True
                ).start()
                self._receive_loop()
            except Exception as e:
                logger.error(f"Could not connect to Go backend ({self.backend_url}): {e}")
            finally:
                cycle_stop.set()

            if self._stop.is_set():
                break
            logger.info(f"Retrying Go backend connection in {self.reconnect_delay:.1f}s ...")
            time.sleep(self.reconnect_delay)

    def stop(self) -> None:
        self._stop.set()
        with self.send_lock:
            if self.ws:
                try:
                    self.ws.close()
                except Exception:
                    pass


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="ROS2 bridge WebSocket client: translates between the Go "
        "backend's /ws/bridge protocol and rosbridge_server."
    )
    parser.add_argument(
        "--backend-url",
        default=os.environ.get("BACKEND_BRIDGE_WS_URL", DEFAULT_BACKEND_WS_URL),
        help="Go backend bridge WebSocket URL (env: BACKEND_BRIDGE_WS_URL)",
    )
    parser.add_argument(
        "--rosbridge-url",
        default=os.environ.get("ROSBRIDGE_WS_URL", DEFAULT_ROSBRIDGE_WS_URL),
        help="rosbridge_server WebSocket URL (env: ROSBRIDGE_WS_URL)",
    )
    parser.add_argument(
        "--reconnect-delay",
        type=float,
        default=float(os.environ.get("BACKEND_BRIDGE_RECONNECT_DELAY", DEFAULT_RECONNECT_DELAY)),
        help="Seconds to wait between reconnect attempts to the Go backend.",
    )
    parser.add_argument(
        "--object-list-refresh-interval",
        type=float,
        default=float(
            os.environ.get(
                "BACKEND_BRIDGE_OBJECT_REFRESH_INTERVAL", DEFAULT_OBJECT_LIST_REFRESH_INTERVAL
            )
        ),
        help="Seconds between workspace object list refreshes sent to the Go backend.",
    )
    parser.add_argument(
        "--log-level",
        default=os.environ.get("BRIDGE_LOG_LEVEL", "INFO"),
        help="Python logging level (e.g. DEBUG, INFO, WARNING).",
    )
    # Use parse_known_args so this also tolerates ROS2-injected arguments
    # (e.g. --ros-args) when started via `ros2 run`/`ros2 launch`.
    args, _unknown = parser.parse_known_args(argv)

    logging.basicConfig(
        level=getattr(logging, str(args.log_level).upper(), logging.INFO),
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    )

    rosbridge = RosbridgeConnection(args.rosbridge_url)
    client = BackendBridgeClient(
        args.backend_url,
        rosbridge,
        reconnect_delay=args.reconnect_delay,
        object_list_refresh_interval=args.object_list_refresh_interval,
    )

    try:
        client.run_forever()
    except KeyboardInterrupt:
        logger.info("Interrupted, shutting down.")
    finally:
        client.stop()


if __name__ == "__main__":
    main()
