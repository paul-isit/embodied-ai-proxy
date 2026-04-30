from src.frontend.tui_app import EmbodiedProxyApp
from src.backend.llm_proxy import LLMProxy

def main():


    proxy = LLMProxy()
    app = EmbodiedProxyApp(proxy)
    app.run()

if __name__ == "__main__":
    main()
