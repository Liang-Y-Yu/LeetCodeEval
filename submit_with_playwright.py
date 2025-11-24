#!/usr/bin/env python3
import json
import sys
import time
from pathlib import Path
from playwright.sync_api import sync_playwright

# Create logs directory
Path("logs").mkdir(exist_ok=True)

def submit_problem(page, problem_file, model_name):
    with open(problem_file) as f:
        data = json.load(f)
    
    # Check if already successfully submitted
    submission = data.get("Submissions", {}).get(model_name, {})
    status = submission.get("CheckResponse", {}).get("status_msg")
    if status == "Accepted":
        print(f"Skipping {problem_file} - already accepted")
        return True
    
    # Get problem details
    url = data["Question"]["Url"]
    code = data["Solutions"][model_name]["TypedCode"]
    title = data["Question"]["Data"]["Question"]["Title"]
    
    print(f"\nSubmitting: {title}")
    print(f"URL: {url}")
    
    # Navigate to problem with retry
    max_retries = 3
    for attempt in range(max_retries):
        try:
            page.goto(url, wait_until="domcontentloaded", timeout=60000)
            break
        except Exception as e:
            if attempt < max_retries - 1:
                print(f"Connection failed, retrying ({attempt + 1}/{max_retries})...")
                time.sleep(5)
            else:
                print(f"Error: {e}")
                return False
    
    # Wait for editor to be ready
    page.wait_for_selector('.monaco-editor', timeout=30000)
    
    # Select Python3 if needed
    try:
        page.evaluate("""
            () => {
                const allButtons = Array.from(document.querySelectorAll('button'));
                const langButton = allButtons.find(btn => {
                    const text = btn.textContent || '';
                    return text.match(/^(C\\+\\+|Python|Java|JavaScript|C#|Ruby|Swift|Go|Kotlin)$/);
                });
                
                if (langButton && !langButton.textContent.includes('Python')) {
                    langButton.click();
                    setTimeout(() => {
                        const options = Array.from(document.querySelectorAll('[role="option"]'));
                        const python3Option = options.find(opt => opt.textContent.includes('Python3'));
                        if (python3Option) python3Option.click();
                    }, 500);
                }
            }
        """)
        time.sleep(1)  # Brief wait for language switch
    except:
        pass
    
    # Clear and input code - use direct JavaScript injection to avoid clipboard issues
    try:
        # Escape the code properly for JavaScript
        escaped_code = code.replace('\\', '\\\\').replace('`', '\\`').replace('$', '\\$')
        
        # Inject code directly into Monaco editor
        page.evaluate(f"""
            () => {{
                const editor = monaco.editor.getModels()[0];
                if (editor) {{
                    editor.setValue(`{escaped_code}`);
                }}
            }}
        """)
        time.sleep(1)
        print("Code injected")
    except Exception as e:
        print(f"Error injecting code: {e}")
        return False
    
    # Submit
    page.click('button:has-text("Submit")')
    print("Submitted, waiting for result...")
    
    # Wait for result to appear - try multiple strategies
    result = None
    try:
        # Strategy 1: Wait for submission result panel (most common)
        page.wait_for_selector('[data-e2e-locator="submission-result"]', timeout=45000)
        time.sleep(2)
    except:
        try:
            # Strategy 2: Wait for any result tab to appear
            page.wait_for_selector('[role="tab"]:has-text("Accepted"), [role="tab"]:has-text("Wrong Answer"), [role="tab"]:has-text("Runtime Error")', timeout=10000)
            time.sleep(2)
        except:
            # Strategy 3: Just wait and hope for the best
            print("Waiting 20 seconds for result...")
            time.sleep(20)
    
    # Take screenshot
    page.screenshot(path=f"logs/debug_{problem_file.stem}.png")
    print(f"Screenshot saved: logs/debug_{problem_file.stem}.png")
    
    # Get result from multiple possible locations
    result = page.evaluate("""
        () => {
            // Method 1: Check submission result panel
            const resultPanel = document.querySelector('[data-e2e-locator="submission-result"]');
            if (resultPanel) {
                const text = resultPanel.innerText;
                if (text.includes('Accepted')) return 'Accepted';
                if (text.includes('Wrong Answer')) return 'Wrong Answer';
                if (text.includes('Time Limit Exceeded')) return 'Time Limit Exceeded';
                if (text.includes('Runtime Error')) return 'Runtime Error';
                if (text.includes('Memory Limit Exceeded')) return 'Memory Limit Exceeded';
                if (text.includes('Compile Error')) return 'Compile Error';
            }
            
            // Method 2: Check tab titles (for errors shown in tabs)
            const tabs = document.querySelectorAll('[role="tab"]');
            for (const tab of tabs) {
                const text = tab.innerText;
                if (text === 'Runtime Error') return 'Runtime Error';
                if (text === 'Wrong Answer') return 'Wrong Answer';
                if (text === 'Accepted') return 'Accepted';
                if (text === 'Time Limit Exceeded') return 'Time Limit Exceeded';
                if (text === 'Memory Limit Exceeded') return 'Memory Limit Exceeded';
            }
            
            // Method 3: Check for status text anywhere on page
            const allText = document.body.innerText;
            if (allText.includes('Runtime Error')) return 'Runtime Error';
            if (allText.includes('Compile Error')) return 'Compile Error';
            if (allText.includes('Time Limit Exceeded')) return 'Time Limit Exceeded';
            if (allText.includes('Memory Limit Exceeded')) return 'Memory Limit Exceeded';
            if (allText.includes('Wrong Answer')) return 'Wrong Answer';
            if (allText.includes('Accepted')) return 'Accepted';
            
            return null;
        }
    """)
    
    if result:
        print(f"Result: {result}")
        accepted = "Accepted" in result
        
        # Update JSON file
        if accepted or "Wrong Answer" in result or "Time Limit" in result or "Runtime Error" in result:
            with open(problem_file) as f:
                data = json.load(f)
            
            data["Submissions"][model_name] = {
                "SubmitRequest": {
                    "lang": "python3",
                    "question_id": data["Question"]["Data"]["Question"]["questionId"],
                    "typed_code": code
                },
                "SubmissionId": 0,
                "CheckResponse": {
                    "status_code": 10 if accepted else 11,
                    "status_msg": "Accepted" if accepted else result.split('\n')[0],
                    "Finished": True,
                    "State": "SUCCESS" if accepted else "FAILED"
                },
                "SubmittedAt": time.strftime("%Y-%m-%dT%H:%M:%S+01:00")
            }
            
            with open(problem_file, 'w') as f:
                json.dump(data, f, indent=2)
            print(f"Updated {problem_file}")
        
        return accepted
    else:
        print("Could not get result, check manually")
        return False

def main():
    if len(sys.argv) < 2:
        print("Usage: ./submit_with_playwright.py <model_name>")
        sys.exit(1)
    
    model_name = sys.argv[1]
    problems_dir = Path("problems")
    
    # Find problems needing submission
    failed_problems = []
    for problem_file in problems_dir.glob("*.json"):
        with open(problem_file) as f:
            data = json.load(f)
        
        if model_name not in data.get("Solutions", {}):
            continue
        
        submission = data.get("Submissions", {}).get(model_name, {})
        status = submission.get("CheckResponse", {}).get("status_msg")
        finished = submission.get("CheckResponse", {}).get("Finished")
        
        # Include problems that are not submitted, or have errors
        if not status or not finished or status in ["Runtime Error", "Wrong Answer", "Time Limit Exceeded", "Memory Limit Exceeded", "Compile Error"]:
            failed_problems.append(problem_file)
    
    print(f"Found {len(failed_problems)} problems to submit")
    
    with sync_playwright() as p:
        browser = p.chromium.launch_persistent_context(
            user_data_dir="./playwright_profile",
            headless=False,
            args=['--disable-blink-features=AutomationControlled']
        )
        page = browser.pages[0] if browser.pages else browser.new_page()
        
        # Hide webdriver
        page.add_init_script("""
            Object.defineProperty(navigator, 'webdriver', {get: () => undefined});
        """)
        
        # Verify login
        page.goto("https://leetcode.com")
        print("\nVerify you're logged in, then press Enter...")
        input()
        
        # Submit each problem
        for i, problem_file in enumerate(failed_problems, 1):
            print(f"\n[{i}/{len(failed_problems)}]")
            try:
                submit_problem(page, problem_file, model_name)
                time.sleep(15)  # Delay between submissions
            except Exception as e:
                print(f"Error: {e}")
                continue
        
        browser.close()
    
    print("\nDone!")

if __name__ == "__main__":
    main()
