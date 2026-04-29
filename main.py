from src.frontend.tui_app import EmbodiedProxyApp
from src.backend.llm_proxy import LLMProxy

def main():
    # add logic here to pass in the path to the configs/ directory
    # and initialize the LLMProxy class like this:
    # proxy = LLMProxy(path_to_configs_directory)
    proxy = LLMProxy()
    app = EmbodiedProxyApp(proxy)
    app.run()

if __name__ == "__main__":
    main()
