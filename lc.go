package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"

	cloudflarebp "github.com/DaRealFreak/cloudflare-bp-go"
	browser "github.com/EDDYCJY/fake-useragent"
	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/chrome"
	_ "github.com/browserutils/kooky/browser/firefox"
	"github.com/rs/zerolog/log"
)

var cookieJarCache *cookiejar.Jar
var leetcodeUrl *url.URL

func init() {
	u, err := url.Parse("https://leetcode.com/")
	if err != nil {
		panic(fmt.Sprintf("failed to parse leetcode url: %v. This is a bug", err))
	}
	leetcodeUrl = u
}

func makeNiceReferer(urlStr string) (string, error) {
	url, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	// remove the last fragment of the path
	url.Path = path.Dir(path.Dir(url.Path + "/"))
	return url.String(), nil
}

func makeAuthorizedHttpRequest(method string, url string, reqBody io.Reader) ([]byte, int, error) {
	log.Trace().Msgf("%s %s", method, url)
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	c := client()
	req.Header = newHeader()
	if referer, err := makeNiceReferer(url); err != nil {
		log.Err(err).Msg("failed to make a referer")
	} else {
		req.Header.Add("Referer", referer)
	}
	req.Header.Add("Content-Length", fmt.Sprintf("%d", req.ContentLength))

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

func cookieJar() http.CookieJar {
	// nil means cookies was never loaded
	if cookieJarCache != nil {
		log.Trace().Msg("using cached cookie jar")
		return cookieJarCache
	}
	loadedJar, err := loadCookieJar()
	if err != nil {
		// if loading of cookies failed, we don't want to make more unsuccessful attempts
		// instead we log the error and cache an empty jar (note that empty is not nil)
		log.Err(err).Msg("failed to get the cookie jar")
		cookieJarCache, _ = cookiejar.New(nil)
		return cookieJarCache
	}
	loadedJarTyped, ok := loadedJar.(*cookiejar.Jar)
	if !ok {
		log.Err(err).Msg("loaded cookie jar is not a cookie jar (this is a bug)")
		cookieJarCache, _ = cookiejar.New(nil)
	}
	cookieJarCache = loadedJarTyped

	return cookieJarCache
}

func loadCookieJarFromFile() (http.CookieJar, error) {
	// First check if the cookie is provided as an environment variable
	sessionCookie := os.Getenv("LEETCODE_SESSION")
	csrfToken := os.Getenv("LEETCODE_CSRF_TOKEN")
	
	if sessionCookie != "" {
		log.Debug().Msg("Using LEETCODE_SESSION environment variable")
		
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create cookie jar: %w", err)
		}
		
		cookies := []*http.Cookie{
			{
				Name:   "LEETCODE_SESSION",
				Value:  sessionCookie,
				Path:   "/",
				Domain: ".leetcode.com",
			},
		}
		
		// Add CSRF token if available
		if csrfToken != "" {
			log.Debug().Msg("Using LEETCODE_CSRF_TOKEN environment variable")
			cookies = append(cookies, &http.Cookie{
				Name:   "csrftoken",
				Value:  csrfToken,
				Path:   "/",
				Domain: ".leetcode.com",
			})
		}
		
		jar.SetCookies(leetcodeUrl, cookies)
		log.Debug().Msg("Successfully loaded cookie from environment variables")
		return jar, nil
	}
	
	// If no environment variable, try to read from file
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	
	cookieFile := homeDir + "/.config/leetcode/cookie"
	if ok, _ := fileExists(cookieFile); !ok {
		return nil, fmt.Errorf("leetcode cookie file not found: %s", cookieFile)
	}
	
	cookieData, err := os.ReadFile(cookieFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read cookie file: %w", err)
	}
	
	cookieValue := strings.TrimSpace(string(cookieData))
	log.Debug().Msgf("Read cookie from file: %s (length: %d)", cookieFile, len(cookieValue))
	
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}
	
	cookies := []*http.Cookie{
		{
			Name:   "LEETCODE_SESSION",
			Value:  cookieValue,
			Path:   "/",
			Domain: ".leetcode.com",
		},
	}
	
	// Check for CSRF token file
	csrfFile := homeDir + "/.config/leetcode/csrf"
	if ok, _ := fileExists(csrfFile); ok {
		csrfData, err := os.ReadFile(csrfFile)
		if err == nil {
			csrfValue := strings.TrimSpace(string(csrfData))
			log.Debug().Msgf("Read CSRF token from file: %s (length: %d)", csrfFile, len(csrfValue))
			cookies = append(cookies, &http.Cookie{
				Name:   "csrftoken",
				Value:  csrfValue,
				Path:   "/",
				Domain: ".leetcode.com",
			})
		}
	}
	
	jar.SetCookies(leetcodeUrl, cookies)
	log.Debug().Msg("Successfully loaded cookie from file")
	return jar, nil
}

func loadCookieJar() (http.CookieJar, error) {
	// First try to load from file
	fileJar, err := loadCookieJarFromFile()
	if err == nil {
		log.Debug().Msg("Successfully loaded cookie from file")
		return fileJar, nil
	}
	log.Debug().Err(err).Msg("Failed to load cookie from file, trying browser cookies")
	
	// If file loading fails, try browser cookies
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to read chrome cookies: %w", err)
	}
	cookiesFile := dir + "/Google/Chrome/Default/Cookies"
	if ok, _ := fileExists(cookiesFile); !ok {
		return nil, fmt.Errorf("chrome cookies file not found or invalid: %s", cookiesFile)
	}

	for _, cookieStore := range kooky.FindAllCookieStores() {
		if ok, _ := fileExists(cookieStore.FilePath()); !ok {
			// skip non-existing cookie stores
			continue
		}
		log.Trace().Msgf("Found cookie store for %s: %s (default: %v)", cookieStore.Browser(), cookieStore.FilePath(), cookieStore.IsDefaultProfile())
		if cookieStore.Browser() == "firefox" && cookieStore.IsDefaultProfile() {
			// modern chrome cookies are not supported by kooky
			subJar, err := cookieStore.SubJar(kooky.Valid)
			defer cookieStore.Close()

			if err != nil {
				log.Err(err).Msgf("failed to get cookie sub jar from %s", cookieStore.FilePath())
				continue
			}
			log.Debug().Msgf("using cookie jar from %s", cookieStore.FilePath())
			return subJar, nil
		}
	}

	return nil, fmt.Errorf("no cookie stores found")
}

func cookie(name string) (*http.Cookie, error) {
	jar := cookieJar()
	cookies := jar.Cookies(leetcodeUrl)
	idx := slices.IndexFunc(cookies, func(c *http.Cookie) bool { return c.Name == name })
	if idx == -1 {
		return nil, nil
	}
	return cookies[idx], nil
}

func newHeader() http.Header {
	cookie, _ := cookie("csrftoken")
	token := ""
	if cookie != nil {
		token = cookie.Value
	}

	return http.Header{
		"Content-Type": {"application/json"},
		"User-Agent":   {browser.Firefox()},
		"X-Csrftoken":  {token},
	}
}

func client() *http.Client {
	client := http.DefaultClient
	client.Transport = newTransport()
	client.Jar = cookieJar()

	return client
}

// &http.Transport{} bypasses cloudflare generally better than DefaultTransport
func newTransport() http.RoundTripper {
	return cloudflarebp.AddCloudFlareByPass(&http.Transport{})
}

func SubmitUrl(q Question) string {
	return leetcodeUrl.Scheme + "://" + leetcodeUrl.Host + "/problems/" + q.Data.Question.TitleSlug + "/submit/"
}

func SubmissionCheckUrl(submissionId uint64) string {
	return leetcodeUrl.Scheme + "://" + leetcodeUrl.Host + "/submissions/detail/" + fmt.Sprint(submissionId) + "/check/"
}
