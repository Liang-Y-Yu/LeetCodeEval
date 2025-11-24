package main

import (
	"context"
	"fmt"
	"strings"
	"time"
	leetgptsolver "whisk/leetgptsolver/pkg"

	"github.com/rs/zerolog/log"
)

// Agent represents a specialized problem-solving agent
type Agent interface {
	Name() string
	Process(ctx context.Context, input AgentInput) (*AgentOutput, error)
}

// AgentInput contains the input data for an agent
type AgentInput struct {
	Question     Question
	PreviousWork map[string]*AgentOutput // Results from previous agents
	ModelName    string
	ModelParams  string
}

// AgentOutput contains the output from an agent
type AgentOutput struct {
	AgentName    string                 `json:"agent_name"`
	Content      string                 `json:"content"`
	Metadata     map[string]interface{} `json:"metadata"`
	ProcessedAt  time.Time              `json:"processed_at"`
	Latency      time.Duration          `json:"latency"`
	PromptTokens int                    `json:"prompt_tokens"`
	OutputTokens int                    `json:"output_tokens"`
}

// Agent implementations
type ProblemAnalyzerAgent struct {
	prompter func(Question, string, string) (*Solution, error)
}

type SolutionDesignerAgent struct {
	prompter func(Question, string, string) (*Solution, error)
}

type CodeExecutorAgent struct {
	prompter func(Question, string, string) (*Solution, error)
}

type SolutionVerifierAgent struct {
	prompter func(Question, string, string) (*Solution, error)
}

// ProblemAnalyzerAgent analyzes the problem and identifies key patterns, constraints, and approaches
func (a *ProblemAnalyzerAgent) Name() string {
	return "problem_analyzer"
}

func (a *ProblemAnalyzerAgent) Process(ctx context.Context, input AgentInput) (*AgentOutput, error) {
	prompt := generateAnalyzerPrompt(input.Question)

	// Create a temporary question for the prompter
	tempQuestion := input.Question
	tempQuestion.Data.Question.Content = prompt

	t0 := time.Now()
	solution, err := a.prompter(tempQuestion, input.ModelName, input.ModelParams)
	latency := time.Since(t0)

	if err != nil {
		return nil, fmt.Errorf("analyzer agent failed: %w", err)
	}

	return &AgentOutput{
		AgentName: a.Name(),
		Content:   solution.Answer,
		Metadata: map[string]interface{}{
			"analysis_type": "problem_breakdown",
		},
		ProcessedAt:  time.Now(),
		Latency:      latency,
		PromptTokens: solution.PromptTokens,
		OutputTokens: solution.OutputTokens,
	}, nil
}

// SolutionDesignerAgent designs the high-level algorithm and approach
func (a *SolutionDesignerAgent) Name() string {
	return "solution_designer"
}

func (a *SolutionDesignerAgent) Process(ctx context.Context, input AgentInput) (*AgentOutput, error) {
	analysis := ""
	if prev, ok := input.PreviousWork["problem_analyzer"]; ok {
		analysis = prev.Content
	}

	prompt := generateDesignerPrompt(input.Question, analysis)

	// Create a temporary question for the prompter
	tempQuestion := input.Question
	tempQuestion.Data.Question.Content = prompt

	t0 := time.Now()
	solution, err := a.prompter(tempQuestion, input.ModelName, input.ModelParams)
	latency := time.Since(t0)

	if err != nil {
		return nil, fmt.Errorf("designer agent failed: %w", err)
	}

	return &AgentOutput{
		AgentName: a.Name(),
		Content:   solution.Answer,
		Metadata: map[string]interface{}{
			"design_type": "algorithm_design",
		},
		ProcessedAt:  time.Now(),
		Latency:      latency,
		PromptTokens: solution.PromptTokens,
		OutputTokens: solution.OutputTokens,
	}, nil
}

// CodeExecutorAgent implements the actual code based on the design
func (a *CodeExecutorAgent) Name() string {
	return "code_executor"
}

func (a *CodeExecutorAgent) Process(ctx context.Context, input AgentInput) (*AgentOutput, error) {
	analysis := ""
	design := ""

	if prev, ok := input.PreviousWork["problem_analyzer"]; ok {
		analysis = prev.Content
	}
	if prev, ok := input.PreviousWork["solution_designer"]; ok {
		design = prev.Content
	}

	selectedSnippet, selectedLang := input.Question.FindSnippet(PREFERRED_LANGUAGES)
	if selectedSnippet == "" {
		return nil, fmt.Errorf("failed to find code snippet for preferred languages")
	}

	prompt := generateExecutorPrompt(input.Question, analysis, design, selectedSnippet, selectedLang)

	// Create a temporary question for the prompter
	tempQuestion := input.Question
	tempQuestion.Data.Question.Content = prompt

	t0 := time.Now()
	solution, err := a.prompter(tempQuestion, input.ModelName, input.ModelParams)
	latency := time.Since(t0)

	if err != nil {
		return nil, fmt.Errorf("executor agent failed: %w", err)
	}

	return &AgentOutput{
		AgentName: a.Name(),
		Content:   solution.Answer,
		Metadata: map[string]interface{}{
			"language":    selectedLang,
			"code_length": len(extractCode(solution.Answer)),
		},
		ProcessedAt:  time.Now(),
		Latency:      latency,
		PromptTokens: solution.PromptTokens,
		OutputTokens: solution.OutputTokens,
	}, nil
}

// SolutionVerifierAgent reviews and validates the solution
func (a *SolutionVerifierAgent) Name() string {
	return "solution_verifier"
}

func (a *SolutionVerifierAgent) Process(ctx context.Context, input AgentInput) (*AgentOutput, error) {
	analysis := ""
	design := ""
	code := ""

	if prev, ok := input.PreviousWork["problem_analyzer"]; ok {
		analysis = prev.Content
	}
	if prev, ok := input.PreviousWork["solution_designer"]; ok {
		design = prev.Content
	}
	if prev, ok := input.PreviousWork["code_executor"]; ok {
		code = extractCode(prev.Content)
	}

	prompt := generateVerifierPrompt(input.Question, analysis, design, code)

	// Create a temporary question for the prompter
	tempQuestion := input.Question
	tempQuestion.Data.Question.Content = prompt

	t0 := time.Now()
	solution, err := a.prompter(tempQuestion, input.ModelName, input.ModelParams)
	latency := time.Since(t0)

	if err != nil {
		return nil, fmt.Errorf("verifier agent failed: %w", err)
	}

	return &AgentOutput{
		AgentName: a.Name(),
		Content:   solution.Answer,
		Metadata: map[string]interface{}{
			"verification_type": "solution_review",
		},
		ProcessedAt:  time.Now(),
		Latency:      latency,
		PromptTokens: solution.PromptTokens,
		OutputTokens: solution.OutputTokens,
	}, nil
}

// Multi-agent orchestrator
func promptMultiAgent(args []string, modelName string) {
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

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
	case leetgptsolver.MODEL_FAMILY_OPENROUTER:
		prompter = promptOpenRouter
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

	// Initialize agents
	agents := []Agent{
		&ProblemAnalyzerAgent{prompter: prompter},
		&SolutionDesignerAgent{prompter: prompter},
		&CodeExecutorAgent{prompter: prompter},
		&SolutionVerifierAgent{prompter: prompter},
	}

	log.Info().Msgf("Processing %d problems with multi-agent approach...", len(files))

	solvedCnt := 0
	skippedCnt := 0
	errorsCnt := 0

	for i, file := range files {
		log.Info().Msgf("[%d/%d] Multi-agent processing %s...", i+1, len(files), file)

		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			errorsCnt += 1
			log.Err(err).Msg("Failed to read the problem")
			continue
		}

		// Check if already solved (same format as prompt.go)
		if _, ok := problem.Solutions[modelName]; ok && !options.Force {
			skippedCnt += 1
			log.Info().Msgf("Already solved at %s", problem.Solutions[modelName].SolvedAt.String())
			continue
		}

		// Process through all agents
		ctx := context.Background()
		agentOutputs := make(map[string]*AgentOutput)
		totalLatency := time.Duration(0)

		input := AgentInput{
			Question:     problem.Question,
			PreviousWork: agentOutputs,
			ModelName:    modelId,
			ModelParams:  modelParams,
		}

		success := true
		for _, agent := range agents {
			log.Info().Msgf("Running %s agent...", agent.Name())

			output, err := agent.Process(ctx, input)
			if err != nil {
				log.Err(err).Msgf("Agent %s failed", agent.Name())
				success = false
				break
			}

			agentOutputs[agent.Name()] = output
			input.PreviousWork = agentOutputs
			totalLatency += output.Latency

			log.Debug().Msgf("Agent %s completed in %v", agent.Name(), output.Latency)
		}

		if !success {
			errorsCnt += 1
			continue
		}

		// Extract final code from executor agent
		finalCode := ""
		if executorOutput, ok := agentOutputs["code_executor"]; ok {
			finalCode = extractCode(executorOutput.Content)
		}

		// Get language from executor metadata
		language := "python3"
		if executorOutput, ok := agentOutputs["code_executor"]; ok {
			if lang, ok := executorOutput.Metadata["language"].(string); ok {
				language = lang
			}
		}

		// Create a combined prompt from all agents (for audit trail)
		combinedPrompt := fmt.Sprintf("Multi-agent approach:\n1. Analysis\n2. Design\n3. Implementation\n4. Verification")

		// Create a combined answer from all agents
		combinedAnswer := ""
		if analyzerOutput, ok := agentOutputs["problem_analyzer"]; ok {
			combinedAnswer += "=== ANALYSIS ===\n" + analyzerOutput.Content + "\n\n"
		}
		if designerOutput, ok := agentOutputs["solution_designer"]; ok {
			combinedAnswer += "=== DESIGN ===\n" + designerOutput.Content + "\n\n"
		}
		if executorOutput, ok := agentOutputs["code_executor"]; ok {
			combinedAnswer += "=== IMPLEMENTATION ===\n" + executorOutput.Content + "\n\n"
		}
		if verifierOutput, ok := agentOutputs["solution_verifier"]; ok {
			combinedAnswer += "=== VERIFICATION ===\n" + verifierOutput.Content + "\n\n"
		}

		// Create solution in the same format as prompt.go
		solution := Solution{
			Lang:         language,
			Prompt:       combinedPrompt,
			Answer:       combinedAnswer,
			TypedCode:    finalCode,
			Model:        modelName,
			SolvedAt:     time.Now(),
			Latency:      totalLatency,
			PromptTokens: sumTokens(agentOutputs, "prompt"),
			OutputTokens: sumTokens(agentOutputs, "output"),
		}

		// Save solution in the same format as prompt.go
		problem.Solutions[modelName] = solution
		problem.Submissions[modelName] = Submission{} // Clear old submissions

		err = problem.SaveProblemInto(file)
		if err != nil {
			errorsCnt += 1
			log.Err(err).Msg("Failed to save the solution")
			continue
		}

		log.Info().Msgf("Got %d line(s) of code in %0.1f second(s)", len(strings.Split(finalCode, "\n")), totalLatency.Seconds())
		solvedCnt += 1
	}

	log.Info().Msgf("Files processed: %d", len(files))
	log.Info().Msgf("Skipped problems: %d", skippedCnt)
	log.Info().Msgf("Problems solved successfully: %d", solvedCnt)
	log.Info().Msgf("Errors: %d", errorsCnt)
}

// Helper function to sum tokens across agents
func sumTokens(outputs map[string]*AgentOutput, tokenType string) int {
	total := 0
	for _, output := range outputs {
		if tokenType == "prompt" {
			total += output.PromptTokens
		} else {
			total += output.OutputTokens
		}
	}
	return total
}

// Prompt generation functions for each agent
func generateAnalyzerPrompt(q Question) string {
	question := htmlToPlaintext(q.Data.Question.Content)

	return fmt.Sprintf(`You are a Problem Analyzer Agent. Your job is to thoroughly analyze this LeetCode problem and provide insights that will help other agents solve it effectively.

Problem: %s

Please analyze this problem and provide:

1. **Problem Type Classification**: What category does this problem belong to? (e.g., Array, String, Tree, Graph, Dynamic Programming, etc.)

2. **Key Patterns & Algorithms**: What algorithmic patterns or techniques are most relevant? (e.g., Two Pointers, Sliding Window, BFS/DFS, etc.)

3. **Constraints Analysis**: What are the time/space complexity requirements based on the constraints?

4. **Edge Cases**: What edge cases should be considered?

5. **Input/Output Analysis**: What is the expected input format and output format?

6. **Difficulty Assessment**: What makes this problem challenging?

Provide a clear, structured analysis that will guide the solution design.`, question)
}

func generateDesignerPrompt(q Question, analysis string) string {
	question := htmlToPlaintext(q.Data.Question.Content)

	return fmt.Sprintf(`You are a Solution Designer Agent. Based on the problem analysis, design a high-level algorithmic approach.

Problem: %s

Analysis from Problem Analyzer:
%s

Please design a solution approach by providing:

1. **Algorithm Choice**: What specific algorithm or approach should be used?

2. **Step-by-Step Approach**: Break down the solution into clear steps

3. **Data Structures**: What data structures are needed?

4. **Time Complexity**: What will be the time complexity of your approach?

5. **Space Complexity**: What will be the space complexity?

6. **Pseudocode**: Provide high-level pseudocode for the solution

Focus on creating a clear, implementable design that addresses all the insights from the analysis.`, question, analysis)
}

func generateExecutorPrompt(q Question, analysis, design, snippet, language string) string {
	question := htmlToPlaintext(q.Data.Question.Content)

	return fmt.Sprintf(`You are a Code Executor Agent. Implement the designed solution in %s.

Problem: %s

Analysis: %s

Design: %s

Code Template:
%s

Please implement the solution by:

1. Following the designed approach exactly
2. Using the provided function signature
3. Writing clean, efficient code
4. Adding necessary helper functions if needed
5. Ensuring the solution handles all edge cases mentioned in the analysis

Requirements:
- Output only valid %s code
- Do not change the function signature
- No comments or explanations in the code
- Make sure the code is syntactically correct and ready to run

Implement the complete solution now:`, language, question, analysis, design, snippet, language)
}

func generateVerifierPrompt(q Question, analysis, design, code string) string {
	question := htmlToPlaintext(q.Data.Question.Content)

	return fmt.Sprintf(`You are a Solution Verifier Agent. Review the implemented solution for correctness and quality.

Problem: %s

Analysis: %s

Design: %s

Implemented Code:
%s

Please verify the solution by checking:

1. **Correctness**: Does the code correctly implement the designed algorithm?

2. **Edge Cases**: Does it handle all the edge cases identified in the analysis?

3. **Complexity**: Does it meet the expected time and space complexity?

4. **Code Quality**: Is the code clean, readable, and following best practices?

5. **Test Cases**: Walk through the provided examples - does the code produce correct outputs?

6. **Potential Issues**: Are there any bugs, logical errors, or improvements needed?

If you find any issues, suggest specific fixes. If the solution is correct, confirm its validity.

Provide your verification report:`, question, analysis, design, code)
}
