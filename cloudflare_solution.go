package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// Enhanced HTTP client with better Cloudflare bypass
func createEnhancedClient() *http.Client {
	// Create custom transport with better TLS configuration
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
			MaxVersion:         tls.VersionTLS13,
			CipherSuites: []uint16{
				tls.TLS_AES_128_GCM_SHA256,
				tls.TLS_AES_256_GCM_SHA384,
				tls.TLS_CHACHA20_POLY1305_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			},
		},
		DisableKeepAlives:     false,
		DisableCompression:    false,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		Jar:       cookieJar(),
	}

	return client
}

// Enhanced headers with more browser-like behavior
func createEnhancedHeaders() http.Header {
	cookie, _ := cookie("csrftoken")
	token := ""
	if cookie != nil {
		token = cookie.Value
	}

	return http.Header{
		"Accept":                       {"application/json, text/plain, */*"},
		"Accept-Encoding":              {"gzip, deflate, br"},
		"Accept-Language":              {"en-US,en;q=0.9"},
		"Cache-Control":                {"no-cache"},
		"Connection":                   {"keep-alive"},
		"Content-Type":                 {"application/json"},
		"DNT":                          {"1"},
		"Origin":                       {"https://leetcode.com"},
		"Pragma":                       {"no-cache"},
		"Sec-Ch-Ua":                   {`"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`},
		"Sec-Ch-Ua-Mobile":            {"?0"},
		"Sec-Ch-Ua-Platform":          {`"Linux"`},
		"Sec-Fetch-Dest":              {"empty"},
		"Sec-Fetch-Mode":              {"cors"},
		"Sec-Fetch-Site":              {"same-origin"},
		"User-Agent":                   {"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"},
		"X-Csrftoken":                  {token},
		"X-Requested-With":            {"XMLHttpRequest"},
	}
}

// Enhanced request function with retry logic and human-like behavior
func makeEnhancedAuthorizedHttpRequest(method string, url string, reqBody io.Reader) ([]byte, int, error) {
	log.Trace().Msgf("%s %s", method, url)
	
	// Add random delay to simulate human behavior
	delay := time.Duration(rand.Intn(3)+2) * time.Second
	log.Debug().Msgf("Waiting %v before request", delay)
	time.Sleep(delay)

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	// Use enhanced client and headers
	c := createEnhancedClient()
	req.Header = createEnhancedHeaders()
	
	if referer, err := makeNiceReferer(url); err != nil {
		log.Err(err).Msg("failed to make a referer")
	} else {
		req.Header.Set("Referer", referer)
	}
	
	// Set content length if we have a body
	if reqBody != nil {
		if bodyBytes, ok := reqBody.(*bytes.Reader); ok {
			req.Header.Set("Content-Length", fmt.Sprintf("%d", bodyBytes.Len()))
		}
	}

	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to do the request: %w", err)
	}
	log.Trace().Msgf("http response %s", resp.Status)

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read response body: %w", err)
	}
	log.Trace().Msgf("got %d bytes body", len(respBody))
	
	if resp.StatusCode != http.StatusOK {
		return respBody, resp.StatusCode, fmt.Errorf("non-ok http response code: %d", resp.StatusCode)
	}
	return respBody, resp.StatusCode, nil
}
