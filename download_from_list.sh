#!/bin/bash

# Script to download LeetCode problems from a list file
# Usage: ./download_from_list.sh experiments/problems_2025_July.txt

INPUT_FILE=$1
BATCH_SIZE=10  # Number of problems to download in each batch
DELAY=1        # Delay between batches in seconds

if [ -z "$INPUT_FILE" ]; then
    echo "Please provide the path to the input file"
    echo "Usage: ./download_from_list.sh experiments/problems_2025_July.txt"
    exit 1
fi

if [ ! -f "$INPUT_FILE" ]; then
    echo "File not found: $INPUT_FILE"
    exit 1
fi

# Create problems directory if it doesn't exist
mkdir -p problems

# Extract problem slugs from the file, skipping comment lines
PROBLEMS=$(grep -v "^#" "$INPUT_FILE" | sed -E 's/problems\/(.*)\.json/\1/')

# Count total problems
TOTAL=$(echo "$PROBLEMS" | wc -l)
echo "Found $TOTAL problems to download"

# Download problems in batches
COUNT=0
BATCH_COUNT=0
BATCH=""

# Use process substitution instead of pipe to avoid subshell variable issues
while read -r slug; do
    if [ -z "$slug" ]; then
        continue
    fi
    
    # Add to current batch
    if [ -z "$BATCH" ]; then
        BATCH="$slug"
    else
        BATCH="$BATCH $slug"
    fi
    
    COUNT=$((COUNT + 1))
    BATCH_COUNT=$((BATCH_COUNT + 1))
    
    # Process batch when it reaches the batch size or at the end
    if [ $BATCH_COUNT -eq $BATCH_SIZE ] || [ $COUNT -eq $TOTAL ]; then
        echo "Downloading batch $(((COUNT - 1) / BATCH_SIZE + 1)) ($COUNT/$TOTAL problems)"
        go run . download -P $BATCH
        
        # Reset batch
        BATCH=""
        BATCH_COUNT=0
        
        # Add delay between batches to avoid rate limiting
        if [ $COUNT -lt $TOTAL ]; then
            echo "Waiting $DELAY seconds before next batch..."
            sleep $DELAY
        fi
    fi
done < <(echo "$PROBLEMS")

echo "Download complete. Downloaded $COUNT problems."
