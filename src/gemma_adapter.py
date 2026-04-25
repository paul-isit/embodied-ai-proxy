import torch
from transformers import AutoModelForCausalLM, AutoTokenizer

class GemmaAdapter:
    def __init__(self):
        self.model_id = "google/gemma-3n-e2b-it"
        print("INFO: Loading Clean Adapter V4...")
        self.tokenizer = AutoTokenizer.from_pretrained(self.model_id)
        self.model = AutoModelForCausalLM.from_pretrained(
            self.model_id,
            device_map="cpu",
            torch_dtype=torch.bfloat16
        )

    def generate(self, prompt):
        inputs = self.tokenizer(prompt, return_tensors="pt").to("cpu")
        try:
            outputs = self.model.generate(**inputs, max_new_tokens=64, do_sample=False)
            text = self.tokenizer.decode(outputs[0][inputs.input_ids.shape[1]:], skip_special_tokens=True).strip()

            print(f"DEBUG: Model Raw Output -> |{text}|")

            start = text.find("{")
            end = text.find("}") + 1

            if start != -1 and end > 0:
                return {"text": text[start:end]}
            return {"text": '{"action": "base_stop"}'}
        except Exception as e:
            return {"text": f"ERROR: {str(e)}"}
