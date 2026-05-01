from src.frontend.tui_app import EmbodiedProxyApp
from src.backend.llm_proxy import LLMProxy

def main():

    config_path = "configs/"
    proxy = LLMProxy(config_path=config_path)
    app = EmbodiedProxyApp(proxy)
    app.run()

if __name__ == "__main__":
    main()
