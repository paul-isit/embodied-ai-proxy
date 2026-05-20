class BaseLLMAdapter:
    def __init__(self, config):
        self.config = config

    def generate(self, prompt: str) -> str:
        raise NotImplementedError(
            f"'{self.__class__.__name__}' must implement a generate() method."
        )