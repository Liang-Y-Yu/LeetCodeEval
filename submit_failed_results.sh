#!/bin/bash

# Script to submit problems that have failed submissions
# Usage: ./submit_failed_results.sh <model_name>
# Example: ./submit_failed_results.sh openai/gpt-5.1

if [ -z "$1" ]; then
    echo "Error: Model name is required"
    echo "Usage: $0 <model_name>"
    echo "Example: $0 openai/gpt-5.1"
    exit 1
fi

MODEL="$1"
BATCH_SIZE=1            # Submit one at a time to avoid rate limiting
DELAY=30                # 30 second delay between submissions
SUBMIT_RETRIES=10       # Increase submit retries
CHECK_RETRIES=15        # Increase check retries

# Set up environment variables for LeetCode authentication
export LEETCODE_SESSION="$(cat ~/.config/leetcode/cookie)"
export LEETCODE_CSRF_TOKEN="$(cat ~/.config/leetcode/csrf)"

# Find problems that have solutions but failed submissions
echo "Finding problems with failed or incomplete submissions..."
FAILED_PROBLEMS=()

for file in problems/*.json; do
    # Check if the problem has a solution for the specified model
    has_solution=$(jq -r ".Solutions[\"$MODEL\"] != null" "$file")
    
    if [ "$has_solution" == "true" ]; then
        # Check if the problem has a successful submission result
        status_msg=$(jq -r ".Submissions[\"$MODEL\"].CheckResponse.status_msg" "$file")
        finished=$(jq -r ".Submissions[\"$MODEL\"].CheckResponse.Finished" "$file")
        
        if [ "$status_msg" == "" ] || [ "$status_msg" == "null" ] || [ "$finished" == "false" ]; then
            FAILED_PROBLEMS+=("$file")
            echo "Found failed submission: $file (Status: '$status_msg', Finished: $finished)"
        fi
    fi
done

# Count total problems with failed results
TOTAL=${#FAILED_PROBLEMS[@]}
echo "Found $TOTAL problems with failed or incomplete submissions"

# Process problems in batches
COUNT=0
BATCH_COUNT=0
BATCH=""

for problem in "${FAILED_PROBLEMS[@]}"; do
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
        
        # Run the submission with increased retries and force flag
        go run . submit -m $MODEL --submit_retries=$SUBMIT_RETRIES --check_retries=$CHECK_RETRIES -f $BATCH
        
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

# Check the actual results after submission attempts
echo "Checking final submission results..."
SUCCESSFUL=0
STILL_FAILED=0

for problem in "${FAILED_PROBLEMS[@]}"; do
    status_msg=$(jq -r ".Submissions[\"$MODEL\"].CheckResponse.status_msg" "$problem")
    finished=$(jq -r ".Submissions[\"$MODEL\"].CheckResponse.Finished" "$problem")
    
    if [ "$status_msg" != "" ] && [ "$status_msg" != "null" ] && [ "$finished" == "true" ]; then
        if [ "$status_msg" == "Accepted" ]; then
            SUCCESSFUL=$((SUCCESSFUL + 1))
        else
            STILL_FAILED=$((STILL_FAILED + 1))
        fi
    else
        STILL_FAILED=$((STILL_FAILED + 1))
    fi
done

# Provide appropriate completion message based on results
if [ $SUCCESSFUL -eq $COUNT ]; then
    echo "Submission complete."
elif [ $SUCCESSFUL -eq 0 ]; then
    echo "Submission complete. Attempted to submit $COUNT problems with failed results."
else
    echo "Submission complete. Successfully submitted $SUCCESSFUL/$COUNT problems ($STILL_FAILED still failed)."
fi
