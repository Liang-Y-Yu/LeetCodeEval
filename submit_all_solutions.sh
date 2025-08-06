#!/bin/bash

# Script to submit all solutions for downloaded problems
# Usage: ./submit_all_solutions.sh <model_name>

MODEL="${1:-se-gpt-4o}"  # Default to se-gpt-4o if no model specified
BATCH_SIZE=5            # Number of problems to process in each batch
DELAY=10                # Delay between batches in seconds

if [ -z "$MODEL" ]; then
    echo "Please provide a model name"
    echo "Usage: ./submit_all_solutions.sh <model_name>"
    exit 1
fi

# Set up environment variables for LeetCode authentication
export LEETCODE_SESSION="$(cat ~/.config/leetcode/cookie)"
export LEETCODE_CSRF_TOKEN="$(cat ~/.config/leetcode/csrf)"

# Get all problem files into an array
PROBLEMS=(problems/*.json)

# Count total problems
TOTAL=${#PROBLEMS[@]}
echo "Found $TOTAL problems to submit solutions for"

# Process problems in batches
COUNT=0
BATCH_COUNT=0
BATCH=""
SUBMITTED=0
SKIPPED=0
ERRORS=0

for problem in "${PROBLEMS[@]}"; do
    if [ ! -f "$problem" ]; then
        continue
    fi
    
    # Check if the problem has a solution for the specified model
    has_solution=$(jq -r ".Solutions[\"$MODEL\"] != null" "$problem")
    if [ "$has_solution" != "true" ]; then
        echo "Skipping $problem - no solution for model $MODEL"
        SKIPPED=$((SKIPPED + 1))
        continue
    fi
    
    # Add to current batch
    if [ -z "$BATCH" ]; then
        BATCH="$problem"
    else
        BATCH="$BATCH $problem"
    fi
    
    COUNT=$((COUNT + 1))
    BATCH_COUNT=$((BATCH_COUNT + 1))
    
    # Process batch when it reaches the batch size or at the end
    if [ $BATCH_COUNT -eq $BATCH_SIZE ] || [ $COUNT -eq $TOTAL ]; then
        echo "Submitting solutions for batch $((COUNT / BATCH_SIZE + 1)) ($COUNT/$TOTAL problems)"
        
        # Run the submission and capture the output
        OUTPUT=$(go run . submit -m $MODEL $BATCH 2>&1)
        echo "$OUTPUT"
        
        # Count submissions and errors
        SUBMITTED_BATCH=$(echo "$OUTPUT" | grep -o "Problems submitted successfully: [0-9]*" | awk '{print $4}')
        ERRORS_BATCH=$(echo "$OUTPUT" | grep -o "Errors: [0-9]*" | awk '{print $2}')
        
        if [ -n "$SUBMITTED_BATCH" ]; then
            SUBMITTED=$((SUBMITTED + SUBMITTED_BATCH))
        fi
        
        if [ -n "$ERRORS_BATCH" ]; then
            ERRORS=$((ERRORS + ERRORS_BATCH))
        fi
        
        # Reset batch
        BATCH=""
        BATCH_COUNT=0
        
        # Add delay between batches to avoid rate limiting
        if [ $COUNT -lt $TOTAL ]; then
            echo "Waiting $DELAY seconds before next batch..."
            sleep $DELAY
        fi
    fi
done

echo "Submission complete."
echo "Total problems processed: $COUNT"
echo "Successfully submitted: $SUBMITTED"
echo "Skipped (no solution): $SKIPPED"
echo "Errors: $ERRORS"
