package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
	"whisk/leetgptsolver/pkg/throttler"

	"github.com/andybalholm/brotli"
	"github.com/rs/zerolog/log"
)

// decompressResponse handles LeetCode's specific compression format
func decompressResponse(data []byte) ([]byte, error) {
	// Check if data is empty
	if len(data) < 2 {
		return data, nil
	}
	
	// Try gzip first (0x1f, 0x8b)
	if data[0] == 0x1f && data[1] == 0x8b {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err == nil {
			defer reader.Close()
			if decompressed, err := io.ReadAll(reader); err == nil {
				return decompressed, nil
			}
		}
	}
	
	// Try brotli decompression (common for modern web responses)
	brotliReader := brotli.NewReader(bytes.NewReader(data))
	if decompressed, err := io.ReadAll(brotliReader); err == nil && len(decompressed) > 0 {
		trimmed := bytes.TrimSpace(decompressed)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			log.Debug().Msgf("Successfully decompressed with brotli: %d -> %d bytes", len(data), len(decompressed))
			return decompressed, nil
		}
	}
	
	// Try zlib format
	if zlibReader, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
		defer zlibReader.Close()
		if decompressed, err := io.ReadAll(zlibReader); err == nil && len(decompressed) > 0 {
			trimmed := bytes.TrimSpace(decompressed)
			if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
				log.Debug().Msgf("Successfully decompressed with zlib: %d -> %d bytes", len(data), len(decompressed))
				return decompressed, nil
			}
		}
	}
	
	// Try raw deflate decompression (no wrapper)
	if deflateReader := flate.NewReader(bytes.NewReader(data)); deflateReader != nil {
		defer deflateReader.Close()
		if decompressed, err := io.ReadAll(deflateReader); err == nil && len(decompressed) > 0 {
			trimmed := bytes.TrimSpace(decompressed)
			if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
				log.Debug().Msgf("Successfully decompressed with raw deflate: %d -> %d bytes", len(data), len(decompressed))
				return decompressed, nil
			}
		}
	}
	
	// If all decompression attempts fail, return original data
	log.Debug().Msgf("All decompression attempts failed, returning original data")
	return data, nil
}

type InvalidCodeError struct {
	error
}

func NewInvalidCodeError(err error) error {
	return InvalidCodeError{err}
}

var leetcodeThrottler throttler.Throttler

func submit(args []string, modelName string) {
	if options.DryRun {
		log.Warn().Msg("Running in dry-run mode. No changes will be made to problem files")
	}
	files, err := filenamesFromArgs(args)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get files")
		return
	}

	log.Info().Msgf("Submitting %d solutions...", len(files))
	submittedCnt := 0
	skippedCnt := 0
	errorsCnt := 0
	// 2 seconds seems to be minimum acceptable delay for leetcode
	leetcodeThrottler = throttler.NewSimpleThrottler(2*time.Second, 60*time.Second)
outerLoop:
	for i, file := range files {
		log.Info().Msgf("[%d/%d] Submitting problem %s ...", i+1, len(files), file)

		var problem Problem
		err := problem.ReadProblem(file)
		if err != nil {
			log.Err(err).Msg("Failed to read problem")
			errorsCnt += 1
			continue
		}

		solv, ok := problem.Solutions[modelName]
		if !ok {
			log.Warn().Msgf("Model %s has no solution to submit", modelName)
			skippedCnt += 1
			continue
		}
		if solv.TypedCode == "" {
			log.Error().Msgf("Model %s has empty solution", modelName)
			skippedCnt += 1
			continue
		}
		subm, ok := problem.Submissions[modelName]
		if !options.Force && (ok && subm.CheckResponse.Finished) {
			log.Info().Msgf("%s's solution is already submitted", modelName)
			skippedCnt += 1
			continue
		}
		log.Info().Msgf("Submitting %s's solution...", modelName)
		submission, err := submitAndCheckSolution(problem.Question, solv)
		if err != nil {
			errorsCnt += 1
			if _, ok := err.(FatalError); ok {
				log.Err(err).Msgf("Aborting...")
				break outerLoop
			}
			log.Err(err).Msgf("Failed to submit or check %s's solution", modelName)
			continue
		}

		log.Info().Msgf("Submission status: %s", submission.CheckResponse.StatusMsg)
		problem.Submissions[modelName] = *submission
		if !options.DryRun {
			err = problem.SaveProblemInto(file)
			if err != nil {
				log.Err(err).Msg("Failed to save the submission result")
				errorsCnt += 1
				continue
			}
		}
		submittedCnt += 1
	}
	log.Info().Msgf("Files processed: %d", len(files))
	log.Info().Msgf("Skipped problems: %d", skippedCnt)
	log.Info().Msgf("Problems submitted successfully: %d", submittedCnt)
	log.Info().Msgf("Errors: %d", errorsCnt)
}

func submitAndCheckSolution(q Question, s Solution) (*Submission, error) {
	subReq := SubmitRequest{
		Lang:       s.Lang,
		QuestionId: q.Data.Question.Id,
		TypedCode:  codeToSubmit(s, true),
	}

	submissionId, err := submitCode(SubmitUrl(q), subReq)
	if err != nil {
		var subErr InvalidCodeError
		if errors.As(err, &subErr) {
			// non-retriable submission error, like "Your code is too long"
			return &Submission{
				SubmitRequest: subReq,
				CheckResponse: CheckResponse{
					StatusMsg:  subErr.Error(),
					Finished:  true,
				},
				SubmittedAt: time.Now(),
			}, nil
		}

		return nil, err
	}

	checkResponse, err := checkStatus(SubmissionCheckUrl(submissionId))
	if err != nil {
		return nil, err
	}

	return &Submission{
		SubmitRequest: subReq,
		SubmissionId:  submissionId,
		CheckResponse: *checkResponse,
		SubmittedAt:   time.Now(),
	}, nil
}

func submitCode(url string, subReq SubmitRequest) (uint64, error) {
	var reqBody bytes.Buffer
	// use encoder, not standard json.Marshal() because we don't need to escape "<", ">" etc. in the source code
	encoder := json.NewEncoder(&reqBody)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(subReq)
	if err != nil {
		return 0, NewNonRetriableError(fmt.Errorf("failed marshaling GraphQL: %w", err))
	}
	log.Trace().Msgf("Submission request body:\n%s", reqBody.String())
	var respBody []byte
	maxRetries := options.SubmitRetries
	i := 0
	leetcodeThrottler.Ready()
	for leetcodeThrottler.Wait() && i < maxRetries {
		i += 1

		// Add delay to appear more human-like
		time.Sleep(time.Duration(5+rand.Intn(5)) * time.Second)

		var code int
		respBody, code, err = makeEnhancedAuthorizedHttpRequest("POST", url, &reqBody)
		leetcodeThrottler.Touch()
		log.Trace().Msgf("submission response body:\n%s", string(respBody))
		if code == http.StatusBadRequest || code == 499 {
			log.Err(err).Msg("Slowing down...")
			leetcodeThrottler.Slowdown()
			err_message := string(respBody)
			if len(err_message) > 80 {
				err_message = err_message[:80] + "..."
			}
			return 0, NewNonRetriableError(fmt.Errorf("invalid or unauthorized request, see response: %s", err_message))
		}
		if code == 403 || code == http.StatusTooManyRequests || err != nil {
			log.Err(err).Msgf("Retrying (%d/%d)...", i, maxRetries)
			leetcodeThrottler.Slowdown()
			time.Sleep(time.Duration(i*10) * time.Second)
			continue
		}

		break // success
	}
	if err != nil {
		return 0, err
	}

	var respStruct map[string]any
	decoder := json.NewDecoder(bytes.NewReader(respBody))
	decoder.UseNumber()
	err = decoder.Decode(&respStruct)
	log.Trace().Msgf("submission response struct: %#v", respStruct)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal submission response: %w", err)
	}
	if errorMsg, ok := respStruct["error"].(string); ok && respStruct["error"] == "Your code is too long. Please reduce your code size and try again." {
		return 0, fmt.Errorf("submission error: %w", NewInvalidCodeError(errors.New(errorMsg)))
	}
	submissionNumber, ok := respStruct["submission_id"].(json.Number);
	if !ok {
		return 0, fmt.Errorf("submission_id is not a number: %v", respStruct["submission_id"])
	}
	submissionId, err := submissionNumber.Int64()
	if err != nil {
		return 0, fmt.Errorf("invalid submission id: %w", err)
	}
	if submissionId <= 0 {
		return 0, fmt.Errorf("invalid submission id: %d", submissionId)
	}
	log.Debug().Msgf("received submission_id: %d", submissionId)

	return uint64(submissionId), nil
}

func checkStatus(url string) (*CheckResponse, error) {
	var checkResp *CheckResponse
	maxRetries := options.CheckRetries
	i := 0
	leetcodeThrottler.Ready()
	for leetcodeThrottler.Wait() && i < maxRetries {
		i += 1
		log.Debug().Msgf("checking submission status (%d/%d)...", i, maxRetries)
		
		// Add progressive delay - start with 3s, increase to 5s after a few retries
		if i > 1 {
			delay := 3 * time.Second
			if i > 3 {
				delay = 5 * time.Second
			}
			log.Debug().Msgf("Waiting %v before checking status...", delay)
			time.Sleep(delay)
		}
		
		respBody, code, err := makeEnhancedAuthorizedHttpRequest("GET", url, bytes.NewReader([]byte{}))
		leetcodeThrottler.Touch()
		
		// Decompress response if it's gzipped
		if len(respBody) >= 2 {
			log.Debug().Msgf("Response starts with: %02x %02x", respBody[0], respBody[1])
		}
		decompressedBody, decompErr := decompressResponse(respBody)
		if decompErr != nil {
			log.Warn().Err(decompErr).Msg("Failed to decompress response, using original")
			decompressedBody = respBody
		} else if len(decompressedBody) != len(respBody) {
			log.Debug().Msgf("Successfully decompressed response from %d to %d bytes", len(respBody), len(decompressedBody))
		}
		
		log.Trace().Msgf("Check response body: %s", string(decompressedBody))
		if code == http.StatusBadRequest || code == 403 || code == 499 {
			err_message := string(decompressedBody)
			if len(err_message) > 80 {
				err_message = err_message[:80] + "..."
			}
			return &CheckResponse{}, NewNonRetriableError(fmt.Errorf("invalid or unauthorized request, see response: %s", err_message))
		}
		if code == http.StatusTooManyRequests || err != nil {
			log.Err(err).Msg("Slowing down...")
			leetcodeThrottler.Slowdown()
			continue
		}

		err = json.Unmarshal(decompressedBody, &checkResp)
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to unmarshal check response (attempt %d/%d)", i, maxRetries)
			continue
		}
		
		// Validate the response has meaningful data
		if checkResp.StatusMsg == "" && !checkResp.Finished {
			log.Warn().Msgf("Received incomplete response (attempt %d/%d), retrying...", i, maxRetries)
			checkResp = nil
			continue
		}
		
		log.Debug().Msgf("Status: %s, Finished: %v, State: %s", checkResp.StatusMsg, checkResp.Finished, checkResp.State)

		if checkResp.Finished {
			log.Info().Msgf("Submission finished with status: %s", checkResp.StatusMsg)
			break // success
		}
		
		log.Debug().Msgf("Submission not finished yet, will retry...")
	}
	if checkResp == nil {
		// did not get a response after retries
		return nil, fmt.Errorf("failed to get check submission status after %d retries", maxRetries)
	}
	if !checkResp.Finished {
		log.Warn().Msgf("Submission check timed out after %d retries. Last status: %s", maxRetries, checkResp.StatusMsg)
		return nil, fmt.Errorf("submission is not finished after %d retries", maxRetries)
	}

	return checkResp, nil
}

func codeToSubmit(s Solution, onlyCode bool) string {
	if onlyCode {
		return s.TypedCode
	}

	return "# leetgptsolver submission\n" +
		fmt.Sprintf("# solution generated by model %s at %s \n", s.Model, s.SolvedAt) +
		s.TypedCode
}
