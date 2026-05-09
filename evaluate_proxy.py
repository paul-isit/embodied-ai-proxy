import argparse
import yaml
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.append(str(ROOT / "src"))

from backend.llm_proxy import LLMProxy

def load_tests(path):
    with open(path, "r") as f:
        data = yaml.safe_load(f)

    return data.get("tests", data)

def extract_actions(recipe: dict):
    return [step.get("action", "").lower() for step in recipe.get("steps", [])]


def validate_basic_structure(recipe):
    if "recipe_name" not in recipe:
        return False
    if "steps" not in recipe:
        return False
    if not isinstance(recipe["steps"], list):
        return False
    return True


def run_tests(proxy, tests):
    results = []

    for i, test in enumerate(tests, 1):
        name = test.get("name", f"test_{i}")
        prompt = test.get("prompt", "")
        objects = test.get("available_objects", [])
        expected_steps = test.get("expected", {}).get("steps", [])
        expected_actions = test.get("expected_actions") or test.get("expected", [])

        print(f"\n[Test {i}] {name}")

        try:
            result = proxy.generate(prompt, objects)
            recipe = result["parsed"]

            if not validate_basic_structure(recipe):
                print("  Result: FAIL (invalid structure)")
                results.append(False)
                continue

            actual_actions = extract_actions(recipe)

            # Compare prefix match (flexible)
            passed = actual_actions[:len(expected_actions)] == expected_actions

            print("  Expected Actions:", expected_actions)
            print("  Actual Actions  :", actual_actions)

            print("  Result:", "PASS" if passed else "FAIL")
            results.append(passed)

        except Exception as e:
            print("  Result: FAIL (Exception)")
            print("  Error:", str(e))
            results.append(False)

    return results
    
def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--config-dir", required=True)
    parser.add_argument("--tests", required=True)

    args = parser.parse_args()

    print("--- Initializing LLM Proxy ---")
    proxy = LLMProxy(args.config_dir)
    print("Proxy initialized successfully.")

    tests = load_tests(args.tests)

    print("\n--- Running Tests ---")
    results = run_tests(proxy, tests)

    passed = sum(results)
    total = len(results)

    print("\n========================================")
    print(" BULK TEST SUMMARY")
    print("========================================")
    print(f" Total Tests Run : {total}")
    print(f" Passed          : {passed}")
    print(f" Failed          : {total - passed}")
    print("========================================")


if __name__ == "__main__":
    main()
