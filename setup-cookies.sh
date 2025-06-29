#!/bin/bash

# Script to set up LeetCode cookies for leetgptsolver in WSL

# Create the directory if it doesn't exist
mkdir -p ~/.config/leetcode

# Function to save a cookie
save_cookie() {
    local name=$1
    local value=$2
    local file=$3
    
    echo -n "$value" > "$file"
    echo "Saved $name cookie to $file"
}

# Check if session cookie value is provided as an argument
if [ -n "$1" ] && [ -n "$2" ]; then
    save_cookie "LEETCODE_SESSION" "$1" ~/.config/leetcode/cookie
    save_cookie "csrftoken" "$2" ~/.config/leetcode/csrf
    echo "Both cookies saved successfully."
    exit 0
fi

# If no arguments provided, prompt the user
echo "Please enter your LeetCode session cookie value:"
echo "You can find this by:"
echo "1. Log in to LeetCode in your browser"
echo "2. Open Developer Tools (F12)"
echo "3. Go to the Application tab"
echo "4. Under Storage > Cookies, find the LeetCode domain"
echo "5. Copy the value of the 'LEETCODE_SESSION' cookie"
echo ""
read -p "LEETCODE_SESSION value: " session_value

if [ -z "$session_value" ]; then
    echo "No session cookie value provided. Exiting."
    exit 1
fi

echo ""
echo "Please enter your LeetCode CSRF token value:"
echo "You can find this by:"
echo "1. In the same cookie list, find the 'csrftoken' cookie"
echo "2. Copy its value"
echo ""
read -p "csrftoken value: " csrf_value

if [ -z "$csrf_value" ]; then
    echo "No CSRF token provided. Exiting."
    exit 1
fi

# Save the cookie values to files
save_cookie "LEETCODE_SESSION" "$session_value" ~/.config/leetcode/cookie
save_cookie "csrftoken" "$csrf_value" ~/.config/leetcode/csrf

echo "You can now use leetgptsolver to submit solutions to LeetCode."
echo ""
echo "Alternatively, you can set environment variables in your shell:"
echo "export LEETCODE_SESSION=\"$session_value\""
echo "export LEETCODE_CSRF_TOKEN=\"$csrf_value\""
