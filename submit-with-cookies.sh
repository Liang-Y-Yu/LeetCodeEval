#!/bin/bash

# Script to submit LeetCode solutions using environment variables for both cookies

# Check if all required arguments are provided
if [ -z "$1" ] || [ -z "$2" ] || [ -z "$3" ]; then
    echo "Usage: $0 <session_cookie> <csrf_token> <model_name> <problem_file>"
    echo "Example: $0 your_session_cookie your_csrf_token se-gpt-4o problems/two-sum.json"
    exit 1
fi

# Check if problem file is provided
if [ -z "$4" ]; then
    echo "Please provide a problem file as the fourth argument"
    echo "Example: $0 your_session_cookie your_csrf_token se-gpt-4o problems/two-sum.json"
    exit 1
fi

# Set the cookie values as environment variables
export LEETCODE_SESSION="$1"
export LEETCODE_CSRF_TOKEN="$2"

# Run the leetgptsolver command with the provided arguments
./leetgptsolver submit -m "$3" "$4"
