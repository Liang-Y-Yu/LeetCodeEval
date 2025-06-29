# LeetCodeEval

Research to evaluate code quality while using prompts in solving coding interview problems.

## Usage

### Supported Models

The tool supports various LLM providers:
- OpenAI
- Azure (GPT-4o)
  - Direct access or via gateway with Azure AD authentication
- Google (Gemini models)
- Anthropic (Claude models)
- DeepSeek
- X.AI (Grok models)

### Configuration

Configure your API keys and endpoints in the `config.yaml` file.

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

### LeetCode Authentication

The tool needs to authenticate with LeetCode to submit solutions. There are two ways to provide authentication:

#### Option 1: Browser Cookies (Default)
The tool will try to read cookies from your browser's cookie store. This works if you're running the tool on the same system where you've logged into LeetCode in your browser.

#### Option 2: Cookie File (For Headless Systems)
If you're running in WSL or a headless environment, you can provide your LeetCode session cookie in a file:

1. Log in to LeetCode in your browser
2. Open Developer Tools (F12)
3. Go to the Application tab
4. Under Storage > Cookies, find the LeetCode domain
5. Copy the value of the "LEETCODE_SESSION" cookie
6. Create a file with this value:
   ```bash
   mkdir -p ~/.config/leetcode
   echo "your_session_cookie_value" > ~/.config/leetcode/cookie
   echo "your_csrf_cookie_value" > ~/.config/leetcode/csrf
   ```

The tool will automatically use this cookie file if it exists and browser cookies are not available.
