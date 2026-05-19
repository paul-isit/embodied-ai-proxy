from rich.text import Text
from textual.widgets import RichLog


class LogPanel(RichLog):

    def on_mount(self):
        self.wrap = True
        self.border_title = "Output"
        self.can_focus = False 

    def add_user(self, text: str):
        self.write(Text.from_markup(f"[bold cyan]=> USER:[/bold cyan] {text}"))

    def add_result(self, result: dict, latency: int, verbosity_level: int = 0):
        """
        Renders robot execution based on CLI verbosity selection.
        """
        self.write(Text.from_markup(f"\n[bold green][RESULT][/bold green] ({latency}ms) "))

        # LEVEL 0 & 1: RECIPE ACTION PARSER 
        if "recipe" in result and result["recipe"]:
            recipe = result.get("recipe", {})
            self.write(Text.from_markup(f"[bold]Recipe:[/bold] {recipe.get('recipe_name', 'unknown')}"))
            
            steps = recipe.get("steps", [])
            if steps:
                self.write(Text.from_markup("Steps:"))
                for step in steps:
                    action_name = step.get('action', '').lower()
                    
                    # Clean Filtering Rule
                    if verbosity_level <= 1 and "gripper" in action_name:
                        continue
                        
                    self.write(Text.from_markup(f"  [yellow]•[/yellow] {step.get('action')}"))
        else:
            self.write(Text.from_markup("[yellow]⚠️ No structured recipe steps returned.[/yellow]"))

        # LEVEL 1 & 2: RAW JSON / STRING STRUCT LOG
        if "raw" in result and verbosity_level >= 1:
            raw = result.get("raw")
            if raw: 
                self.write(Text.from_markup("\n[dim]=== RAW LLM OUTPUT ===[/dim]"))
                self.write(Text(str(raw).strip(), style="italic gray"))

        # LEVEL 2 ONLY: ACTIVE CONTEXT STATE & ENVIRONMENTAL INVENTORY
        if verbosity_level >= 2:
            self.write(Text.from_markup("\n[magenta]=== ENGINE METADATA & WORLD STATE ===[/magenta]"))
            
            # 1. System Pipeline Details
            self.write(Text.from_markup("  [bold cyan]Inference Engine Parameters:[/bold cyan]"))
            self.write(Text.from_markup(f"    • [bold]Target Model:[/bold]      {result.get('model_name', 'Ollama Config Standard')}"))
            self.write(Text.from_markup(f"    • [bold]Endpoint Route:[/bold]    {result.get('endpoint', 'http://localhost:11434')}"))
            self.write(Text.from_markup(f"    • [bold]Schema Status:[/bold]     [bold green]Enforced JSON Structured Mode[/bold green]"))
            
            # 2. Dynamic Object Context Tracker
            self.write(Text.from_markup("\n  [bold cyan]ROS Bridge Environmental Context Map:[/bold cyan]"))
            objects = result.get("objects_mapped", [])
            
            if objects:
                # Count and display currently available physical items
                self.write(Text.from_markup(f"    • [bold]Detected Objects ({len(objects)}):[/bold]"))
                for obj in objects:
                    self.write(Text.from_markup(f"      [yellow]▪[/yellow] {obj}"))
            else:
                #Will be reworked once service with middleware is established
                self.write(Text.from_markup("    • [bold]Detected Objects:[/bold] [yellow] No objects currently registered in ROS cache.[/yellow]"))

        self.write(Text.from_markup("\n" + "─" * 40))

    def add_error(self, msg: str):
        self.write(Text.from_markup(f"[reverse red][ERROR][/reverse red] {msg}"))
