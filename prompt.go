package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	leetgptsolver "whisk/leetgptsolver/pkg"
	"whisk/leetgptsolver/pkg/throttler"

	"cloud.google.com/go/vertexai/genai"
	"github.com/anthropics/anthropic-sdk-go"
	anthropic_option "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/cohesion-org/deepseek-go"
	"github.com/microcosm-cc/bluemonday"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/api/option"
)

var promptThrottler throttler.Throttler

func prompt(args []string, modelName string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	promptThrottler = throttler.NewSimpleThrottler(1*time.Second, 30*time.Second)

	if modelName == "" {
		log.Error().Msg("Model is not set")
		return
	}
	modelId, modelParams, err := leetgptsolver.ParseModelName(modelName)
	if err != nil {
		log.Err(err).Msg("failed to parse model")
		return
	}

	var prompter func(Question, string, string) (*Solution, error)
	switch leetgptsolver.ModelFamily(modelId) {
	case leetgptsolver.MODEL_FAMILY_OPENAI:
		prompter = promptOpenAi
	case leetgptsolver.MODEL_FAMILY_GOOGLE:
		prompter = promptGoogle
	case leetgptsolver.MODEL_FAMILY_ANTHROPIC:
		prompter = promptAnthropic
	case leetgptsolver.MODEL_FAMILY_DEEPSEEK:
		prompter = promptDeepseek
	case leetgptsolver.MODEL_FAMILY_XAI:
		prompter = promptXai
	case leetgptsolver.MODEL_FAMILY_AZURE_OPENAI:
		prompter = promptAzureOpenAi
	default:
		log.Error().Msgf("No prompter found for model %s", modelId)
		return
	}

	log.Info().Msgf("Prompting %d solutions...", len(files))
	solvedCnt := 0
	skippedCnt := 0
	errorsCnt := 0
outerLoop:
	for i, file := range files {
		log.Info().Msgf("[%d/%d] Prompting %s for problem %s ...", i+1, len(files), modelName, file)

		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			errorsCnt += 1
			log.Err(err).Msg("Failed to read the problem")
			continue
		}
		if _, ok := problem.Solutions[modelName]; ok && !options.Force {
			skippedCnt += 1
			log.Info().Msgf("Already solved at %s", problem.Solutions[modelName].SolvedAt.String())
			continue
		}

		var solution *Solution
		maxReties := options.Retries
		i := 0
		promptThrottler.Ready()
		for promptThrottler.Wait() && i < maxReties {
			i += 1
			solution, err = prompter(problem.Question, modelId, modelParams)
			promptThrottler.Touch()
			if err != nil {
				log.Err(err).Msg("Failed to get a solution")
				promptThrottler.Slowdown()
				if _, ok := err.(FatalError); ok {
					log.Error().Msg("Aborting...")
					errorsCnt += 1
					break outerLoop
				}
				if _, ok := err.(NonRetriableError); ok {
					errorsCnt += 1
					continue outerLoop
				}
				// do not retry on this kind of timeout. It usually means the problem takes too much time to solve,
				// and retrying will not help
				if errors.Is(err, context.DeadlineExceeded) {
					errorsCnt += 1
					continue outerLoop
				}
				continue
			}

			break // success
		}

		if solution == nil {
			// did not get a solution after retries
			errorsCnt += 1
			continue
		}

		log.Info().Msgf("Got %d line(s) of code in %0.1f second(s)", strings.Count(solution.TypedCode, "\n"), solution.Latency.Seconds())
		problem.Solutions[modelName] = *solution
		problem.Submissions[modelName] = Submission{} // new solutions clears old submissions
		err = problem.SaveProblemInto(file)
		if err != nil {
			errorsCnt += 1
			log.Err(err).Msg("Failed to save the solution")
			continue
		}

		solvedCnt += 1
	}
	log.Info().Msgf("Files processed: %d", len(files))
	log.Info().Msgf("Skipped problems: %d", skippedCnt)
	log.Info().Msgf("Problems solved successfully: %d", solvedCnt)
	log.Info().Msgf("Errors: %d", errorsCnt)
}

func promptOpenAi(q Question, modelName string, params string) (*Solution, error) {
	client := openai.NewClient(options.ChatgptApiKey)
	lang, prompt, err := generatePrompt(q)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	seed := int(42)
	t0 := time.Now()
	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: modelName,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Seed: &seed,
		},
	)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, NewNonRetriableError(errors.New("no choices in response"))
	}
	answer := resp.Choices[0].Message.Content
	log.Trace().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        resp.Model,
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}

func promptAzureOpenAi(q Question, modelName string, params string) (*Solution, error) {
	var client *openai.Client
	
	if options.UseGateway {
		// Use gateway configuration with Azure AD authentication
		config := openai.DefaultConfig("")
		// Use the correct URL format for the Ericsson gateway
		config.BaseURL = fmt.Sprintf("%s/generativeai-model/v1/azure/openai/deployments/%s", 
			options.GatewayURL, options.AzureModel)
		
		// Create a custom HTTP client with Azure AD authentication
		config.HTTPClient = &http.Client{
			Transport: &azureAuthTransport{
				clientID:     options.AzureClientID,
				clientSecret: options.AzureClientSecret,
				tenantID:     options.AzureTenantID,
				transport:    http.DefaultTransport,
			},
		}
		
		client = openai.NewClientWithConfig(config)
	} else {
		// Use direct Azure OpenAI configuration
		config := openai.DefaultAzureConfig(
			options.AzureOpenAIApiKey,
			options.AzureOpenAIEndpoint,
		)
		
		// Set the API version
		if options.AzureOpenAIApiVersion != "" {
			config.APIVersion = options.AzureOpenAIApiVersion
		}
		
		client = openai.NewClientWithConfig(config)
	}
	
	lang, prompt, err := generatePrompt(q)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	seed := int(42)
	t0 := time.Now()
	
	var req openai.ChatCompletionRequest
	
	if options.UseGateway {
		// For gateway, use the model name directly and add api-version as a query parameter
		req = openai.ChatCompletionRequest{
			Model: "chat", // The model is specified in the URL path, use "chat" here
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Seed: &seed,
		}
	} else {
		// For direct Azure OpenAI, use deployment ID as the model name
		req = openai.ChatCompletionRequest{
			Model: options.AzureOpenAIDeploymentID, // Use deployment ID as the model name for Azure
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Seed: &seed,
		}
	}
	
	resp, err := client.CreateChatCompletion(context.Background(), req)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, NewNonRetriableError(errors.New("no choices in response"))
	}
	answer := resp.Choices[0].Message.Content
	log.Trace().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        resp.Model,
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}

// Custom transport for Azure AD authentication
type azureAuthTransport struct {
	clientID     string
	clientSecret string
	tenantID     string
	transport    http.RoundTripper
	token        string
	tokenExpiry  time.Time
}

func (t *azureAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if we need to refresh the token
	if t.token == "" || time.Now().After(t.tokenExpiry) {
		log.Debug().Msg("Token is empty or expired, refreshing...")
		if err := t.refreshToken(); err != nil {
			log.Error().Err(err).Msg("Failed to refresh token")
			return nil, err
		}
	}
	
	// Add the token to the request
	req.Header.Set("Authorization", "Bearer "+t.token)
	
	// Add Content-Type header
	req.Header.Set("Content-Type", "application/json")
	
	// Add api-version as a query parameter
	q := req.URL.Query()
	q.Add("api-version", options.AzureApiVersion)
	req.URL.RawQuery = q.Encode()
	
	log.Debug().Msgf("Making request to %s with Authorization header", req.URL.String())
	
	// Forward the request to the underlying transport
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		log.Error().Err(err).Msg("Request failed")
		return nil, err
	}
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Debug().Msgf("Request failed with status %s: %s", resp.Status, string(body))
		// Create a new response body with the same content
		resp.Body = io.NopCloser(bytes.NewBuffer(body))
	}
	
	return resp, err
}

func (t *azureAuthTransport) refreshToken() error {
	// Use the format from the Python implementation
	authority := fmt.Sprintf("https://login.microsoftonline.com/%s", t.tenantID)
	tokenURL := fmt.Sprintf("%s/oauth2/token", authority)
	
	// Create form data
	formData := make(map[string]string)
	formData["client_id"] = t.clientID
	formData["client_secret"] = t.clientSecret
	formData["scope"] = fmt.Sprintf("%s/.default", t.clientID)
	formData["grant_type"] = "client_credentials"
	
	// Convert form data to URL-encoded form
	formValues := url.Values{}
	for key, value := range formData {
		formValues.Add(key, value)
	}
	
	log.Debug().Msgf("Requesting token from %s with client_id %s and scope %s", 
		tokenURL, t.clientID, fmt.Sprintf("%s/.default", t.clientID))
	
	// Create request
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(formValues.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	
	// Send request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Debug().Msgf("Token request failed: %s, %s", resp.Status, string(body))
		return fmt.Errorf("failed to get token: %s, %s", resp.Status, string(body))
	}
	
	// Parse response
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   json.Number `json:"expires_in"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return err
	}
	
	// Convert expires_in to int
	expiresIn, err := tokenResp.ExpiresIn.Int64()
	if err != nil {
		return fmt.Errorf("failed to parse expires_in: %w", err)
	}
	
	// Update token and expiry
	t.token = tokenResp.AccessToken
	t.tokenExpiry = time.Now().Add(time.Duration(expiresIn-300) * time.Second) // Refresh 5 minutes before expiry
	log.Debug().Msgf("Got token, expires in %d seconds", expiresIn)
	
	return nil
}

func promptDeepseek(q Question, modelName string, params string) (*Solution, error) {
	client := deepseek.NewClient(options.DeepseekApiKey)
	lang, prompt, err := generatePrompt(q)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	t0 := time.Now()
	client.Timeout = 15 * time.Minute
	resp, err := client.CreateChatCompletion(
		context.Background(),
		&deepseek.ChatCompletionRequest{
			Model: modelName,
			Messages: []deepseek.ChatCompletionMessage{
				{
					Role:    deepseek.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: 0.0,
		},
	)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, NewNonRetriableError(errors.New("no choices in response"))
	}
	answer := resp.Choices[0].Message.Content
	log.Trace().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        resp.Model,
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}

// very dirty
func promptXai(q Question, modelName string, params string) (*Solution, error) {
	config := openai.DefaultConfig(options.XaiApiKey)
	config.BaseURL = "https://api.x.ai/v1"
	client := openai.NewClientWithConfig(config)

	lang, prompt, err := generatePrompt(q)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	var customParams struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if params != "" {
		err = json.Unmarshal([]byte(params), &customParams)
		if err != nil {
			return nil, NewFatalError(fmt.Errorf("failed to parse custom params: %w", err))
		}
		log.Debug().Msgf("using custom params: %+v", customParams)
	}

	seed := int(42)
	completionRequest := openai.ChatCompletionRequest{
		Model: modelName,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Seed: &seed,
	}
	if customParams.ReasoningEffort != "" {
		completionRequest.ReasoningEffort = customParams.ReasoningEffort
	}

	t0 := time.Now()
	resp, err := client.CreateChatCompletion(
		context.Background(),
		completionRequest,
	)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, NewNonRetriableError(errors.New("no choices in response"))
	}
	answer := resp.Choices[0].Message.Content
	log.Trace().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        resp.Model,
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
	}, nil
}

func promptGoogle(q Question, modelName string, params string) (*Solution, error) {
	defer func() {
		if err := recover(); err != nil {
			log.Error().Msgf("recovered: %v", err)
		}
	}()

	lang, prompt, err := generatePrompt(q)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	projectID := options.GeminiProjectId
	region := options.GeminiRegion

	ctx := context.Background()
	opts := option.WithCredentialsFile(options.GeminiCredentialsFile)
	client, err := genai.NewClient(ctx, projectID, region, opts)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to create a client: %w", err))
	}
	defer client.Close()

	gemini := client.GenerativeModel(modelName)
	temp := float32(0.0)
	gemini.GenerationConfig.Temperature = &temp
	chat := gemini.StartChat()
	if chat == nil {
		return nil, errors.New("failed to start a chat")
	}

	t0 := time.Now()
	resp, err := chat.SendMessage(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("failed to send a message: %w", err)
	}
	answer, err := geminiAnswer(resp)
	latency := time.Since(t0)
	if err != nil {
		return nil, err
	}

	log.Trace().Msgf("Got answer:\n%s", answer)
	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        gemini.Name(),
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: int(resp.UsageMetadata.PromptTokenCount),
		OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
	}, nil
}

func promptAnthropic(q Question, modelName string, params string) (*Solution, error) {
	client := anthropic.NewClient(anthropic_option.WithAPIKey(options.ClaudeApiKey))
	lang, prompt, err := generatePrompt(q)
	if err != nil {
		return nil, NewFatalError(fmt.Errorf("failed to make prompt: %w", err))
	}
	log.Debug().Msgf("Generated %d line(s) of code prompt", strings.Count(prompt, "\n"))
	log.Trace().Msgf("Generated prompt:\n%s", prompt)

	var customParams struct {
		MaxTokens int `json:"max_tokens"`
		Thinking  struct {
			Type         string `json:"type"`
			BudgetTokens int    `json:"budget_tokens"`
		}
	}
	if params != "" {
		err = json.Unmarshal([]byte(params), &customParams)
		if err != nil {
			return nil, NewFatalError(fmt.Errorf("failed to parse custom params: %w", err))
		}
		log.Debug().Msgf("using custom params: %+v", customParams)
	}

	messageParams := anthropic.MessageNewParams{
		Model:       modelName,
		Temperature: anthropic.Float(0.0),
		Messages: []anthropic.MessageParam{{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{
				OfRequestTextBlock: &anthropic.TextBlockParam{Text: prompt},
			}},
		}},
		MaxTokens: 4096,
	}
	if customParams.MaxTokens > 0 {
		messageParams.MaxTokens = int64(customParams.MaxTokens)
	}
	if customParams.Thinking.Type == "enabled" {
		messageParams.Thinking = anthropic.ThinkingConfigParamUnion{
			OfThinkingConfigEnabled: &anthropic.ThinkingConfigEnabledParam{
				Type:         "enabled",
				BudgetTokens: int64(customParams.Thinking.BudgetTokens),
			},
		}
		messageParams.Temperature = anthropic.Float(1.0)
	}

	t0 := time.Now()
	resp, err := client.Messages.New(context.Background(), messageParams)
	latency := time.Since(t0)
	if err != nil {
		return nil, fmt.Errorf("failed to send a message: %w", err)
	}

	log.Trace().Msgf("Got response:\n%+v", resp.Content)
	answer := ""
	for _, block := range resp.Content {
		if block.Text != "" {
			answer += block.Text + "\n"
		}
	}

	return &Solution{
		Lang:         lang,
		Prompt:       prompt,
		Answer:       answer,
		TypedCode:    extractCode(answer),
		Model:        modelName,
		SolvedAt:     time.Now(),
		Latency:      latency,
		PromptTokens: int(resp.Usage.InputTokens),
		OutputTokens: int(resp.Usage.OutputTokens),
	}, nil

}

// very hackish
func geminiAnswer(r *genai.GenerateContentResponse) (string, error) {
	var parts []string
	if len(r.Candidates) == 0 {
		return "", NewNonRetriableError(errors.New("no candidates in response"))
	}
	if len(r.Candidates[0].Content.Parts) == 0 && r.Candidates[0].FinishReason == genai.FinishReasonRecitation {
		return "", NewNonRetriableError(errors.New("got FinishReasonRecitation in response"))
	}
	buf, err := json.Marshal(r.Candidates[0].Content.Parts)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(buf, &parts)
	if err != nil {
		return "", err
	}
	return strings.Join(parts, ""), nil
}

func generatePrompt(q Question) (string, string, error) {
	prompt := options.PromptTemplate
	if prompt == "" {
		return "", "", errors.New("prompt_template is not set")
	}

	selectedSnippet, selectedLang := q.FindSnippet(PREFERRED_LANGUAGES)
	if selectedSnippet == "" {
		return "", "", fmt.Errorf("failed to find code snippet for %s", selectedLang)
	}
	question := htmlToPlaintext(q.Data.Question.Content)
	if replaceInplace(&prompt, "{language}", selectedLang) == 0 {
		return "", "", errors.New("no {language} in prompt_template")
	}
	if replaceInplace(&prompt, "{question}", question) == 0 {
		return "", "", errors.New("no {question} in prompt_template")
	}
	if replaceInplace(&prompt, "{snippet}", selectedSnippet) == 0 {
		return "", "", errors.New("no {snippet} in prompt_template")
	}

	return selectedLang, prompt, nil
}

func replaceInplace(s *string, old, new string) int {
	cnt := strings.Count(*s, old)
	*s = strings.ReplaceAll(*s, old, new)
	return cnt
}

func htmlToPlaintext(s string) string {
	// add newlines where necessary
	s = strings.ReplaceAll(s, "<br>", "<br>\n")
	s = strings.ReplaceAll(s, "<br/>", "<br/>\n")
	s = strings.ReplaceAll(s, "</p>", "</p>\n")

	// handle superscript <sup>...</sup>
	s = regexp.MustCompile(`\<sup\>(.*?)\<\/sup\>`).ReplaceAllString(s, "^$1")

	// strip html tags
	p := bluemonday.StrictPolicy()
	s = p.Sanitize(s)

	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#34;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&amp;", "&")

	// collapse multiple newlines
	s = regexp.MustCompile(`\s+$`).ReplaceAllString(s, "")
	s = regexp.MustCompile(`\n+`).ReplaceAllString(s, "\n")

	return s
}

func extractCode(answer string) string {
	re := regexp.MustCompile("(?ms)^```\\w*\\s*$(.+?)^```\\s*$")
	m := re.FindStringSubmatch(answer)
	if m == nil {
		// maybe answer is the code itself?
		return answer
	}
	return m[1]
}
