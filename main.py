import logging
from src.frontend.tui_app import EmbodiedProxyApp
from src.backend.llm_proxy import LLMProxy

def main():
    logging.basicConfig(
        filename='proxy.log',
        level=logging.INFO,
        format='%(asctime)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )

    config_path = "configs/"
    proxy = LLMProxy(config_path=config_path)
    app = EmbodiedProxyApp(proxy)
    app.run()

if __name__ == "__main__":
    main()
