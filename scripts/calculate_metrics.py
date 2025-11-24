import os
import json
import glob

RESULTS_DIR = "/Volumes/ReposCS/liang/LeetCodeEval/results"
AGENT_TYPES = ["single-agent", "multi-agent"]
MODELS = ["gpt-5.1", "claude-4.5", "deepseek-chat-v3.1", "gemini-2.5-pro"]

def calculate_metrics():
    print(f"{'Model':<20} {'Agent Type':<15} {'Avg Latency (s)':<20} {'Avg Tokens':<15}")
    print("-" * 70)

    for model in MODELS:
        for agent_type in AGENT_TYPES:
            base_path = os.path.join(RESULTS_DIR, agent_type, model, "problems")
            json_files = glob.glob(os.path.join(base_path, "*.json"))
            
            total_latency = 0
            total_tokens = 0
            count = 0
            
            for file_path in json_files:
                try:
                    with open(file_path, 'r') as f:
                        data = json.load(f)
                        
                    solutions = data.get("Solutions", {})
                    # The key in solutions might vary slightly (e.g. openai/gpt-5.1 vs just gpt-5.1)
                    # We'll just take the first value found, as there should be only one solution per file for the specific model run
                    if not solutions:
                        continue
                        
                    solution_data = list(solutions.values())[0]
                    
                    latency_ns = solution_data.get("Latency", 0)
                    prompt_tokens = solution_data.get("PromptTokens", 0)
                    output_tokens = solution_data.get("OutputTokens", 0)
                    
                    total_latency += latency_ns
                    total_tokens += (prompt_tokens + output_tokens)
                    count += 1
                except Exception as e:
                    print(f"Error reading {file_path}: {e}")

            if count > 0:
                avg_latency_s = (total_latency / count) / 1e9
                avg_tokens = total_tokens / count
                print(f"{model:<20} {agent_type:<15} {avg_latency_s:<20.2f} {avg_tokens:<15.0f}")
            else:
                print(f"{model:<20} {agent_type:<15} {'N/A':<20} {'N/A':<15}")

if __name__ == "__main__":
    calculate_metrics()
