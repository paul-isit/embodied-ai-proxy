from src.frontend.tui_app import EmbodiedProxyApp
from src.backend.llm_proxy import LLMProxy

def main():
    paths_to_configs_directory = "configs/" #config file directory 
    proxy = LLMProxy(paths_to_configs_directory)
    app = EmbodiedProxyApp(proxy)
    app.run()

if __name__ == "__main__":
    main()
