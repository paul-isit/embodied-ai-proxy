def format_llm_prompt(text: str) -> dict:
    return {
        "prompt": text,
        "mode": "execute"
    }
