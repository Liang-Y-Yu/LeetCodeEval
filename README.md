# LeetCodeEval

A tool for evaluating code quality and performance of Large Language Models (LLMs) on LeetCode coding interview problems.
This repository includes automated problem downloading, solution generation, submission to LeetCode, and metrics analysis.

## <a id="quick-start"></a>🚀 Quick Start

### Prerequisites
- Go 1.19+ installed
- LeetCode account with valid session cookies
- API keys for your chosen LLM provider

### Basic Setup
1. **Clone and build:**
   ```bash
   git clone <repository-url>
   cd LeetCodeEval
   go mod tidy
   ```

2. **Configure API keys** in `config.production` (see [Configuration](#configuration))

3. **Set up LeetCode authentication** (see [LeetCode Authentication](#leetcode-authentication))

4. **Run a complete evaluation manually:**

   ```bash
   # Step-by-step complete evaluation
   go run . download 1 15 71 200                    # Download problems
   go run . prompt --model gpt-4o problems/*.json   # Generate solutions
   ./submit_all_solutions.sh                        # Submit to LeetCode
   ./extract-metrics.sh                             # Analyze results
   ```

## 📋 Table of Contents

- [🚀 Quick Start](#quick-start)
- [⚙️ Configuration](#configuration)
- [🔐 LeetCode Authentication](#leetcode-authentication)
- [🛠️ Core Commands](#core-commands)
- [🤖 Automated Scripts](#automated-scripts)
- [📊 Metrics Analysis](#metrics-analysis)
- [📁 File Structure](#file-structure)
- [🔧 Troubleshooting](#troubleshooting)
- [📈 Example Workflow](#example-workflow)

## <a id="configuration"></a>⚙️ Configuration

### Supported LLM Providers
- **OpenAI** (GPT-4)
- **Azure OpenAI** (GPT-4o with direct access or gateway)
- **DeepSeek**

### Configuration File (`config.production`)

#### For Azure Direct Access:
```yaml
azure_openai_api_key: your-azure-api-key-here
azure_openai_endpoint: https://your-resource-name.openai.azure.com
azure_openai_deployment_id: your-deployment-id-here
azure_openai_api_version: "2024-02-01"
use_gateway: false
```

#### For Azure with Gateway:
```yaml
azure_client_id: your-client-id-here
azure_client_secret: your-client-secret-here
azure_tenant_id: your-tenant-id-here
azure_model: gpt-4o
azure_api_version: "2024-10-21"
use_gateway: true
gateway_url: "https://your-gateway-url.com"
```

## <a id="leetcode-authentication"></a>🔐 LeetCode Authentication

The tool requires LeetCode session cookies to submit solutions.

### Option 1: Browser Cookies (Recommended)

```bash
./setup-cookies.sh
```

### Option 2: Manual Cookie Setup

For headless environments or when browser cookies aren't accessible:

1. **Get your session cookies:**
   - Log in to LeetCode in your browser
   - Open Developer Tools (F12) → Application → Cookies
   - Copy the values for `LEETCODE_SESSION` and `csrftoken`

2. **Create cookie files:**

   ```bash
   mkdir -p ~/.config/leetcode
   echo "your_leetcode_session_value" > ~/.config/leetcode/cookie
   echo "your_csrf_token_value" > ~/.config/leetcode/csrf
   ```

## <a id="core-commands"></a>🛠️ Core Commands

### Download Problems

```bash
# Download specific problems
go run . download 1 15 71 200

# Download problems from a list
./download_from_list.s experiments/problem_list.txt
```

### Generate Solutions

```bash
# Generate solutions for all downloaded problems
go run . prompt --model gpt-4o problems/*.json

# Generate solutions for specific problems
go run . prompt --model gpt-4o problems/two-sum.json problems/3sum.json

# Use different models
go run . prompt --model gemini-pro

# Use multiagents generation
go run . multiagent --model gpt-4o problems/*.json

# Generate with retries
go run . prompt --model gpt-4o --retries 3 problems/*.json
```

### Submit Solutions

```bash
# Submit all solutions (skips already submitted)
go run . submit --model gpt-4o

# Force re-submit all solutions
go run . submit --model gpt-4o -f

# Submit specific problems
go run . submit --model gpt-4o problems/two-sum.json

# Submit with custom retry settings
go run . submit --model gpt-4o --submit_retries=10 --check_retries=15
```

### Extract Metrics

```bash
# Generate comprehensive metrics analysis
./extract-metrics.sh

# Results will be saved in metrics/ folder
```

### Generate Dataset

```bash
# Generate Hugging Face compatible dataset in JSONL format
./generate-dataset.sh > datasets/leetcode_dataset.jsonl

# View dataset summary
cat datasets/leetcode_dataset.jsonl | jq -r '"\(.frontend_id): \(.title) (\(.difficulty))"' | head -10

# Count problems by difficulty
cat datasets/leetcode_dataset.jsonl | jq -r '.difficulty' | sort | uniq -c
```

### List Problems

```bash
# List all downloaded problems
go run . list

# Filter problems by difficulty
go run . list --where '.Question.Data.Question.Difficulty == "Medium"'

# Show only specific fields
go run . list --print '.Question.Data.Question.Title'

# List without header
go run . list --header=false
```

## <a id="automated-scripts"></a>🤖 Automated Scripts

### Complete Evaluation Pipeline

```bash
# Manual step-by-step evaluation (no single script available)
go run . download 1 15 71 200                        # Download problems
go run . prompt --model gpt-4o problems/*.json       # Generate solutions  
./submit_all_solutions.sh                            # Submit to LeetCode
./extract-metrics.sh                                 # Analyze results
```

## <a id="metrics-analysis"></a>📊 Metrics Analysis

The `extract-metrics.sh` script generates comprehensive analysis in the `metrics/` folder:

### Generated Files:

- **`metrics_results.jsonl`** - Detailed metrics for each problem (JSONL format)
- **`metrics_summary.json`** - Aggregated statistics by model
- **`most_complex_problems.json`** - Top 10 most complex problems
- **`failed_problems.json`** - Problems with wrong answers
- **`complexity_distribution.json`** - Problems grouped by complexity ranges

### Key Metrics:

- **Acceptance Rate** - Percentage of solutions that passed all test cases
- **Cyclomatic Complexity** - Code complexity analysis
- **Lines of Code** - Solution length analysis
- **Runtime/Memory Percentiles** - Performance metrics from LeetCode

### Example Analysis:

```bash
# View summary statistics
cat metrics/metrics_summary.json | jq .

# Find most complex problems
cat metrics/most_complex_problems.json | jq '.[0:5]'

# Check failed problems
cat metrics/failed_problems.json | jq '.[] | {title, status}'

# Analyze complexity distribution
cat metrics/complexity_distribution.json | jq '.[] | {range: .complexity_range, count}'

# View dataset summary
cat datasets/leetcode_dataset.jsonl | jq -r '"\(.frontend_id): \(.title) (\(.difficulty))"' | head -10
```

### Dataset Analysis Examples:
```bash
# Count problems by difficulty
cat datasets/leetcode_dataset.jsonl | jq -r '.difficulty' | sort | uniq -c

# Find problems with high acceptance rate
cat datasets/leetcode_dataset.jsonl | jq 'select(.acceptance_rate > 0.7) | {title, acceptance_rate}'

# Get all array-related problems
cat datasets/leetcode_dataset.jsonl | jq 'select(.topic_tags[] == "Array") | .title'

# Extract Python3 code templates
cat datasets/leetcode_dataset.jsonl | jq '.code_snippets[] | select(.lang == "python3") | .code'

# Find problems by category
cat datasets/leetcode_dataset.jsonl | jq -r '.category' | sort | uniq -c
```

## <a id="file-structure"></a>📁 File Structure

```
LeetCodeEval/
├── README.md                          # This file
├── config.yaml                        # LLM API configuration
├── go.mod, go.sum                     # Go dependencies
├── main.go                            # Main entry point
├── download.go                        # Problem downloading logic
├── prompt.go                          # Solution generation logic
├── submit.go                          # Solution submission logic
├── cloudflare_solution.go             # Enhanced HTTP client for Cloudflare bypass
├── lc.go                              # LeetCode API utilities
├── extract-metrics.sh                 # Metrics analysis script
├── submit_all_solutions.sh            # Batch submission script
├── submit_failed_results.sh           # Failed solutions re-submission
├── download_from_list.sh              # Download problems from list
├── generate-dataset.sh                # Generate Hugging Face dataset
├── problems/                          # Downloaded problems (JSON format)
│   ├── two-sum.json
│   ├── 3sum.json
│   └── ...
├── metrics/                           # Generated analysis results
│   ├── metrics_results.jsonl
│   ├── metrics_summary.json
│   ├── most_complex_problems.json
│   ├── failed_problems.json
│   └── complexity_distribution.json
└── datasets/                          # Generated datasets
    └── leetcode_dataset.jsonl         # Hugging Face compatible dataset
```

## <a id="troubleshooting"></a>🔧 Troubleshooting

### Debug Mode

```bash
# Run with verbose logging
go run . submit --model gpt-4o -vvv

# Check specific problem submission
go run . submit --model gpt-4o problems/specific-problem.json -vvv
```

## <a id="example-workflow"></a>📈 Example Workflow

Here's a complete example of evaluating GPT-4o on LeetCode problems:

```bash
# 1. Download a set of problems
./download_from_list.sh experiments/problems_2025_July.txt

# 2. Generate solutions
go run . prompt --model gpt-4o problems/*.json

# 3. Submit solutions to LeetCode
./submit_all_solutions.sh

# 4. Generate comprehensive metrics
./extract-metrics.sh

# 5. Generate Hugging Face dataset
./generate-dataset.sh > datasets/leetcode_dataset.jsonl

# 6. View results
echo "=== Summary Statistics ==="
cat metrics/metrics_summary.json | jq .

echo "=== Most Complex Problems ==="
cat metrics/most_complex_problems.json | jq '.[0:5]'

echo "=== Failed Problems ==="
cat metrics/failed_problems.json | jq '.[] | {title, status}'
```

---

**Note:** This tool is for research purposes.
