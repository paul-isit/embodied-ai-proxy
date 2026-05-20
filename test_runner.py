# test_runner.py
import argparse
import yaml
import json
import logging
from pathlib import Path
from src.backend.llm_proxy import LLMProxy

# ── Argument Parser ───────────────────────────────────────────────
def parse_args():
    """
    Parses command line arguments passed in by the researcher.
    Example:
        python test_runner.py --config configs/ --tests tests/test_cases.yaml
    """
    parser = argparse.ArgumentParser(description="LLM Proxy Test Runner")

    parser.add_argument(
        "--config",
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
        print(f"❌ Test file not found: {path}")
        raise
    except yaml.YAMLError as e:
        print(f"❌ Invalid YAML in test file: {e}")
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
        if step.get("parameters", {}).get("target")
    ]

    # Check first action is home
    if "first_action" in expected:
        if all_actions[0] != expected["first_action"]:
            failures.append(
                f"First action should be '{expected['first_action']}' "
                f"but got '{all_actions[0]}'"
            )

    # Check last action is home
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

    for i, test_case in enumerate(test_cases):
        name   = test_case.get("name", f"Test {i + 1}")
        prompt = test_case.get("prompt", "")

        print(f"Running: {name}")
        print(f"Prompt:  \"{prompt}\"")

        # Run the proxy
        result = proxy.process_user_request(prompt)

        # Print the system prompt used
        print(f"\nSystem Prompt Used:")
        print("-" * 40)
        print(result["prompt"])
        print("-" * 40)

        # Validate the response
        passed, failures = validate_response(test_case, result)

        if passed:
            print(f"✅ PASS\n")
            passed_count += 1
        else:
            print(f"❌ FAIL")
            for failure in failures:
                print(f"   → {failure}")
            print()
            failed_count += 1

        results.append({
            "name": name,
            "passed": passed,
            "failures": failures
        })

    # ── Summary Report ────────────────────────────────────────────
    print("=" * 60)
    print("  SUMMARY")
    print("=" * 60)
    print(f"  Total:  {len(test_cases)}")
    print(f"  ✅ Passed: {passed_count}")
    print(f"  ❌ Failed: {failed_count}")
    print("=" * 60)

    if failed_count > 0:
        print("\nFailed Tests:")
        for r in results:
            if not r["passed"]:
                print(f"  ❌ {r['name']}")
                for failure in r["failures"]:
                    print(f"     → {failure}")
    print()


# ── Entry Point ───────────────────────────────────────────────────
def main():
    # Parse command line arguments
    args = parse_args()

    # Initialise the proxy with the config path
    print(f"Loading config from: {args.config}")
    proxy = LLMProxy(config_path=args.config)

    # Load test cases
    print(f"Loading test cases from: {args.tests}")
    test_cases = load_test_cases(args.tests)
    print(f"Found {len(test_cases)} test cases\n")

    # Run the tests
    run_tests(proxy, test_cases)


if __name__ == "__main__":
    main()