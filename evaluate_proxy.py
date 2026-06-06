import argparse
import yaml
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.append(str(ROOT / "src"))

from backend.llm_proxy import LLMProxy

# ── Argument Parser ───────────────────────────────────────────────
def parse_args():
    """
    Parses command line arguments passed in by the researcher.
    Example:
        python evaluate_proxy.py --config configs/ --tests tests/test_cases.yaml
    """
    parser = argparse.ArgumentParser(description="LLM Proxy Test Runner")

    parser.add_argument(
        "--config-dir",
        type=str,
        required=True,
        help="Path to the directory containing the context config files"
    )
    parser.add_argument(
        "--tests",
        type=str,
        required=True,
        help="Path to the YAML file containing the test cases"
    )

    return parser.parse_args()

# ── Load Test Cases ───────────────────────────────────────────────
def load_test_cases(test_path: str) -> list:
    """
    Loads the YAML test cases file and returns a list of test cases.
    """
    path = Path(test_path)

    try:
        with open(path, "r") as f:
            data = yaml.safe_load(f)
        return data["test_cases"]
    except FileNotFoundError:
        print(f"Test file not found: {path}")
        raise
    except yaml.YAMLError as e:
        print(f"Invalid YAML in test file: {e}")
        raise

# ── Validate Response ─────────────────────────────────────────────
def validate_response(test_case: dict, result: dict) -> tuple[bool, list[str]]:
    """
    Validates the LLM response against the expected values in the test case.
    Returns a tuple of (passed: bool, failures: list of failure messages)
    """
    failures = []
    expected = test_case.get("expected", {})

    # Check if the proxy returned a valid JSON response
    if result["json"] is None:
        failures.append("No JSON returned from LLM")
        return False, failures

    response_json = result["json"]

    # Check recipe has steps
    steps = response_json.get("steps", [])
    if not steps:
        failures.append("No steps found in response")
        return False, failures

    # Get all actions and targets from the steps
    all_actions = [step.get("action") for step in steps]
    all_targets = [
        step.get("parameters", {}).get("target")
        for step in steps
        if (step.get("parameters") or {}).get("target")  # ← handles None parameters
    ]

    # Check first action
    if "first_action" in expected:
        if all_actions[0] != expected["first_action"]:
            failures.append(
                f"First action should be '{expected['first_action']}' "
                f"but got '{all_actions[0]}'"
            )

    # Check last action
    if "last_action" in expected:
        if all_actions[-1] != expected["last_action"]:
            failures.append(
                f"Last action should be '{expected['last_action']}' "
                f"but got '{all_actions[-1]}'"
            )

    # Check required actions are present
    if "must_contain_actions" in expected:
        for action in expected["must_contain_actions"]:
            if action not in all_actions:
                failures.append(f"Missing expected action: '{action}'")

    # Check required targets are present
    if "must_contain_targets" in expected:
        for target in expected["must_contain_targets"]:
            if target not in all_targets:
                failures.append(f"Missing expected target: '{target}'")

    passed = len(failures) == 0
    return passed, failures

# ── Run Tests ─────────────────────────────────────────────────────
def run_tests(proxy: LLMProxy, test_cases: list) -> None:
    """
    Runs all test cases and prints a summary report.
    """
    passed_count = 0
    failed_count = 0
    results = []

    print("\n" + "=" * 60)
    print("  LLM PROXY TEST RUNNER")
    print("=" * 60 + "\n")

    for i, test_case in enumerate(test_cases, 1):
        name    = test_case.get("name", f"Test {i}")
        prompt  = test_case.get("prompt", "")
        objects = test_case.get("available_objects", [])

        print(f"[Test {i}] {name}")
        print(f"Prompt: \"{prompt}\"")

        try:
            # Uses generate() — no ROS2 bridge needed
            result = proxy.generate(prompt, objects)
            wrapped = {"json": result["parsed"] if isinstance(result["parsed"], dict) else result["parsed"].model_dump()}


            steps = wrapped["json"].get("steps", [])
            actual_actions = [step.get("action") for step in steps]
            expected_actions = test_case.get("expected", {}).get("must_contain_actions", [])
            print(f"  Expected Actions : {expected_actions}")
            print(f"  Actual Actions   : {actual_actions}")

            passed, failures = validate_response(test_case, wrapped)

            if passed:
                print(f"  Result: PASS\n")
                passed_count += 1
            else:
                print(f"  Result: FAIL")
                for failure in failures:
                    print(f"    → {failure}")
                print()
                failed_count += 1

        except Exception as e:
            print(f"  Result: FAIL (Exception)")
            print(f"  Error: {str(e)}\n")
            failed_count += 1
            passed = False
            failures = [str(e)]

        results.append({
            "name": name,
            "passed": passed,
            "failures": failures
        })

    # ── Summary Report ────────────────────────────────────────────
    print("=" * 60)
    print("  SUMMARY")
    print("=" * 60)
    print(f"  Total  : {len(test_cases)}")
    print(f"  Passed : {passed_count}")
    print(f"  Failed : {failed_count}")
    print("=" * 60)

    if failed_count > 0:
        print("\nFailed Tests:")
        for r in results:
            if not r["passed"]:
                print(f"   {r['name']}")
                for failure in r["failures"]:
                    print(f"     → {failure}")
    print()

# ── Entry Point ───────────────────────────────────────────────────
def main():
    args = parse_args()

    print(f"Loading config from: {args.config_dir}")
    proxy = LLMProxy(args.config_dir)
    print(f"Proxy initialized successfully.")

    print(f"Loading test cases from: {args.tests}")
    test_cases = load_test_cases(args.tests)
    print(f"Found {len(test_cases)} test cases")

    run_tests(proxy, test_cases)

if __name__ == "__main__":
    main()