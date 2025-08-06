#!/usr/bin/env bash

# Script to extract code quality metrics from LeetCode solutions
# Metrics focus: Correctness, Code Complexity, and Maintainability
# Format: JSON Lines (jsonl)

# Make script executable
chmod +x extract-metrics.sh

# Install required tools if not already installed
command -v gocyclo >/dev/null 2>&1 || go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
command -v gocloc >/dev/null 2>&1 || go install github.com/hhatto/gocloc/cmd/gocloc@latest
command -v dupl >/dev/null 2>&1 || go install github.com/mibk/dupl@latest

# Create temporary directory for code extraction
TEMP_DIR=$(mktemp -d)
echo "Created temporary directory: $TEMP_DIR"

# Function to extract code metrics for a specific language
extract_metrics() {
    local file=$1
    local model=$2
    local lang=$3
    local code=$4
    local file_path="$TEMP_DIR/$(basename "$file" .json)_${model}.${lang}"
    
    # Write code to temporary file
    echo "$code" > "$file_path"
    
    # Calculate metrics based on language
    local loc=$(wc -l < "$file_path")
    
    # Language-specific metrics
    local cyclomatic_complexity=0
    local duplication_score=0
    
    case $lang in
        py|python|python3)
            # Use radon for Python cyclomatic complexity if available
            if command -v radon >/dev/null 2>&1; then
                cyclomatic_complexity=$(radon cc "$file_path" -s -n B | awk '{sum+=$2} END {print sum}')
                # If radon fails or returns empty, set default
                if [ -z "$cyclomatic_complexity" ]; then cyclomatic_complexity=0; fi
            else
                # Fallback: count control flow statements for Python
                cyclomatic_complexity=$(grep -E 'if |elif |while |for |try:|except|and |or |break|continue' "$file_path" | wc -l)
            fi
            
            # Use pylint for duplication detection if available
            if command -v pylint >/dev/null 2>&1; then
                duplication_score=$(pylint --disable=all --enable=duplicate-code "$file_path" 2>&1 | grep -c "Similar lines")
            fi
            ;;
            
        js|javascript)
            # Use complexity-report for JS if available
            if command -v cr >/dev/null 2>&1; then
                cyclomatic_complexity=$(cr "$file_path" -p cyclomatic | grep -oP 'Mean per-function cyclomatic complexity: \K[0-9\.]+')
                if [ -z "$cyclomatic_complexity" ]; then cyclomatic_complexity=0; fi
            fi
            
            # Use jscpd for duplication if available
            if command -v jscpd >/dev/null 2>&1; then
                duplication_score=$(jscpd "$file_path" --silent | grep -oP '[0-9\.]+(?=% duplicated)')
                if [ -z "$duplication_score" ]; then duplication_score=0; fi
            fi
            ;;
            
        go)
            # Use gocyclo for Go cyclomatic complexity
            if command -v gocyclo >/dev/null 2>&1; then
                cyclomatic_complexity=$(gocyclo "$file_path" | awk '{sum+=$1} END {print sum}')
                if [ -z "$cyclomatic_complexity" ]; then cyclomatic_complexity=0; fi
            fi
            
            # Use dupl for Go code duplication
            if command -v dupl >/dev/null 2>&1; then
                duplication_score=$(dupl -t 10 "$file_path" | wc -l)
            fi
            ;;
            
        java)
            # Use PMD for Java metrics if available
            if command -v pmd >/dev/null 2>&1; then
                cyclomatic_complexity=$(pmd check -d "$file_path" -R category/java/design.xml -f text | grep -c "cyclomatic complexity")
                duplication_score=$(pmd cpd --minimum-tokens 50 --files "$file_path" | grep -c "Found a")
            fi
            ;;
            
        *)
            # Generic approach for other languages
            # Count conditional statements as rough complexity estimate
            cyclomatic_complexity=$(grep -E 'if|while|for|switch|case|&&|\|\||catch|\\?|:' "$file_path" | wc -l)
            ;;
    esac
    
    # Return metrics as JSON
    echo "{\"file\":\"$(basename "$file")\", \"model\":\"$model\", \"language\":\"$lang\", \"lines_of_code\":$loc, \"cyclomatic_complexity\":$cyclomatic_complexity, \"duplication_score\":$duplication_score}"
}

# Main processing function
process_file() {
    local file=$1
    local problem_json=$(cat "$file")
    
    # Extract problem ID and title
    local problem_id=$(echo "$problem_json" | jq -r '.Question.Data.Question.questionId')
    local title=$(echo "$problem_json" | jq -r '.Question.Data.Question.Title // .Question.Data.Question.title // "Unknown"')
    
    echo "Processing $file (Problem #$problem_id: $title)..."
    
    # Get all models that have solutions
    local models=$(echo "$problem_json" | jq -r '.Solutions | keys[]')
    
    for model in $models; do
        # Extract solution details
        local solution=$(echo "$problem_json" | jq -r ".Solutions[\"$model\"]")
        local lang=$(echo "$solution" | jq -r '.Lang')
        local code=$(echo "$solution" | jq -r '.TypedCode')
        
        # Skip if no code
        if [ "$code" = "null" ] || [ -z "$code" ]; then
            echo "  Skipping $model (no code)"
            continue
        fi
        
        # Extract correctness metrics
        local submission=$(echo "$problem_json" | jq -r ".Submissions[\"$model\"]")
        local status_msg=$(echo "$submission" | jq -r '.CheckResponse.status_msg // "Not Submitted"')
        local is_accepted=$([ "$status_msg" = "Accepted" ] && echo "true" || echo "false")
        
        # Extract runtime and memory percentiles from submission data
        local runtime_percentile=$(echo "$submission" | jq -r '.CheckResponse.runtime_percentile // 0')
        local memory_percentile=$(echo "$submission" | jq -r '.CheckResponse.memory_percentile // 0')
        
        # Get code metrics
        local code_metrics=$(extract_metrics "$file" "$model" "$lang" "$code")
        
        # Combine all metrics - use compact JSON output for JSONL format
        local all_metrics=$(echo "$code_metrics" | jq -c --arg problem_id "$problem_id" \
                                                  --arg title "$title" \
                                                  --arg status "$status_msg" \
                                                  --arg accepted "$is_accepted" \
                                                  --arg runtime "$runtime_percentile" \
                                                  --arg memory "$memory_percentile" \
                                                  '. + {
                                                      "problem_id": $problem_id,
                                                      "title": $title,
                                                      "status": $status,
                                                      "accepted": ($accepted == "true"),
                                                      "runtime_percentile": ($runtime | tonumber),
                                                      "memory_percentile": ($memory | tonumber)
                                                  }')
        
        echo "$all_metrics" >> metrics/metrics_results.jsonl
        echo "  Processed $model solution"
    done
}

# Create metrics directory
echo "Creating metrics directory..."
mkdir -p metrics

# Initialize output file (empty, no blank line)
echo "Generating metrics dataset..."
> metrics/metrics_results.jsonl

# Process all problem files or specific ones if provided
if [ $# -eq 0 ]; then
    for file in problems/*.json; do
        process_file "$file"
    done
else
    for file in "$@"; do
        process_file "$file"
    done
fi

# Clean up
rm -rf "$TEMP_DIR"
echo "Metrics extraction complete. Results saved to metrics/metrics_results.jsonl"

# Generate summary statistics
echo "Generating summary statistics..."
cat metrics/metrics_results.jsonl | jq -s '
    {
        total_problems: length,
        problems_by_model: group_by(.model) | map({key: .[0].model, value: length}) | from_entries,
        acceptance_rate_by_model: group_by(.model) | map({
            key: .[0].model, 
            value: ((map(select(.accepted == true)) | length) * 100.0 / length)
        }) | from_entries,
        avg_complexity_by_model: group_by(.model) | map({
            key: .[0].model, 
            value: (if length > 0 then (map(.cyclomatic_complexity) | add) / length else 0 end)
        }) | from_entries,
        avg_loc_by_model: group_by(.model) | map({
            key: .[0].model, 
            value: (if length > 0 then (map(.lines_of_code) | add) / length else 0 end)
        }) | from_entries
    }
' > metrics/metrics_summary.json

echo "Summary statistics saved to metrics/metrics_summary.json"

# Generate additional analysis files
echo "Generating additional analysis files..."

# Top 10 most complex problems
cat metrics/metrics_results.jsonl | jq -s 'sort_by(-.cyclomatic_complexity) | .[0:10] | map({title, problem_id, cyclomatic_complexity, lines_of_code, status})' > metrics/most_complex_problems.json

# Failed problems analysis
cat metrics/metrics_results.jsonl | jq -s 'map(select(.accepted == false)) | sort_by(.title)' > metrics/failed_problems.json

# Problems by complexity distribution
cat metrics/metrics_results.jsonl | jq -s '
    group_by(
        if .cyclomatic_complexity == 0 then "0 (Simple)"
        elif .cyclomatic_complexity <= 3 then "1-3 (Low)"
        elif .cyclomatic_complexity <= 7 then "4-7 (Medium)"
        elif .cyclomatic_complexity <= 15 then "8-15 (High)"
        else "16+ (Very High)"
        end
    ) | map({
        complexity_range: .[0] | (
            if .cyclomatic_complexity == 0 then "0 (Simple)"
            elif .cyclomatic_complexity <= 3 then "1-3 (Low)"
            elif .cyclomatic_complexity <= 7 then "4-7 (Medium)"
            elif .cyclomatic_complexity <= 15 then "8-15 (High)"
            else "16+ (Very High)"
            end
        ),
        count: length,
        problems: map(.title)
    })
' > metrics/complexity_distribution.json

echo "Analysis complete! All files saved in the 'metrics' directory:"
echo "  - metrics/metrics_results.jsonl (detailed metrics for each problem)"
echo "  - metrics/metrics_summary.json (aggregated statistics)"
echo "  - metrics/most_complex_problems.json (top 10 most complex problems)"
echo "  - metrics/failed_problems.json (problems with wrong answers)"
echo "  - metrics/complexity_distribution.json (complexity distribution analysis)"
