
import json
import numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.metrics.pairwise import cosine_similarity


class RecipeDatabase:
    """
    Vector database of example JSON schema recipes.
    Converts user prompts into vectors and retrieves
    the most similar recipes.
    """

    def __init__(self):
        # Hardcoded recipes
        self.recipes = self._load_recipes()

        # Extract descriptions for TF‑IDF training
        self.descriptions = [r["description"] for r in self.recipes]

        # Build TF‑IDF vectorizer
        self.vectorizer = TfidfVectorizer()
        self.recipe_vectors = self.vectorizer.fit_transform(self.descriptions)

        print(f"Loaded {len(self.recipes)} recipes into TF‑IDF database")

    def _load_recipes(self):
        """
        Hardcodes the example recipes and adds them to the database.
        Each recipe is stored with its description as the searchable text
        and the full JSON recipe as metadata.
        The descriptions are designed to contain many synonyms and semantic cues to help matching.
        """
        return [
            {
                "id": "recipe_1",
                "description": "pick up an object, pick and place, grab, grasp, lift, raise, hoist, collect object, move object to a location, transport item, carry item, relocate item, drop, release, put down",
                "recipe": {
                    "recipe_name": "Pick and Place Object",
                    "steps": [
                        { "step_id": 1, "action": "home", "description": "Start at home" },
                        { "step_id": 2, "action": "gripper", "parameters": { "position": 1.0 }, "description": "Open gripper" },
                        { "step_id": 3, "action": "move_arm", "parameters": { "target": "target_object" }, "description": "Move to target object" },
                        { "step_id": 4, "action": "gripper", "parameters": { "position": 0.0 }, "description": "Close gripper" },
                        { "step_id": 5, "action": "relative_move", "parameters": { "vector": "move_upwards" }, "description": "Lift object" },
                        { "step_id": 6, "action": "move_arm", "parameters": { "target": "target_location" }, "description": "Move to target location" },
                        { "step_id": 7, "action": "gripper", "parameters": { "position": 1.0 }, "description": "Release object" },
                        { "step_id": 8, "action": "home", "description": "Return to home" }
                    ]
                }
            },
            {
                "id": "recipe_2",
                "description": "move arm to a location, navigate to a position, go to object, reach object, extend arm, reposition arm, travel to point, move toward target, approach object, direct arm to coordinates",
                "recipe": {
                    "recipe_name": "Move Arm to Location",
                    "steps": [
                        { "step_id": 1, "action": "home", "description": "Start at home" },
                        { "step_id": 2, "action": "move_arm", "parameters": { "target": "target_location" }, "description": "Move to target location" },
                        { "step_id": 3, "action": "home", "description": "Return to home" }
                    ]
                }
            },
            {
                "id": "recipe_3",
                "description": "inspect object, examine item, check object, look closely, observe, analyze, study, review, visually inspect, assess object, look at item, investigate object",
                "recipe": {
                    "recipe_name": "Inspect Object",
                    "steps": [
                        { "step_id": 1, "action": "home", "description": "Start at home" },
                        { "step_id": 2, "action": "move_arm", "parameters": { "target": "target_object" }, "description": "Move to object" },
                        { "step_id": 3, "action": "relative_move", "parameters": { "vector": "move_upwards" }, "description": "Lift arm for better view" },
                        { "step_id": 4, "action": "relative_move", "parameters": { "vector": "move_left" }, "description": "Pan left to inspect" },
                        { "step_id": 5, "action": "home", "description": "Return to home" }
                    ]
                }
            },
            {
                "id": "recipe_4",
                "description": "collect multiple objects, gather items, pick up several things, retrieve objects, accumulate items, scoop up objects, gather multiple things, collect items one by one, move multiple objects, transport several items",
                "recipe": {
                    "recipe_name": "Collect Multiple Objects",
                    "steps": [
                        { "step_id": 1, "action": "home", "description": "Start at home" },
                        { "step_id": 2, "action": "gripper", "parameters": { "position": 1.0 }, "description": "Open gripper" },
                        { "step_id": 3, "action": "move_arm", "parameters": { "target": "first_object" }, "description": "Move to first object" },
                        { "step_id": 4, "action": "gripper", "parameters": { "position": 0.0 }, "description": "Close gripper" },
                        { "step_id": 5, "action": "relative_move", "parameters": { "vector": "move_upwards" }, "description": "Lift first object" },
                        { "step_id": 6, "action": "move_arm", "parameters": { "target": "delivery_tray" }, "description": "Place first object" },
                        { "step_id": 7, "action": "gripper", "parameters": { "position": 1.0 }, "description": "Release first object" },
                        { "step_id": 8, "action": "move_arm", "parameters": { "target": "second_object" }, "description": "Move to second object" },
                        { "step_id": 9, "action": "gripper", "parameters": { "position": 0.0 }, "description": "Close gripper" },
                        { "step_id": 10, "action": "relative_move", "parameters": { "vector": "move_upwards" }, "description": "Lift second object" },
                        { "step_id": 11, "action": "move_arm", "parameters": { "target": "delivery_tray" }, "description": "Place second object" },
                        { "step_id": 12, "action": "gripper", "parameters": { "position": 1.0 }, "description": "Release second object" },
                        { "step_id": 13, "action": "home", "description": "Return to home" }
                    ]
                }
            }
        ]

    def query(self, user_prompt: str, n_results: int = 2) -> list[dict]:
        """
        Convert user prompt to TF-IDF vector and return top-N similar recipes.
        """

        query_vec = self.vectorizer.transform([user_prompt])
        similarities = cosine_similarity(query_vec, self.recipe_vectors)[0]

        # Get top-N indices
        top_indices = np.argsort(similarities)[::-1][:n_results]

        return [self.recipes[i]["recipe"] for i in top_indices]


