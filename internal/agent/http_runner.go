package agent

import (
	"net/http"
	"time"
)

// TODO: Прокомментируй
type HTTPRunner struct {
	client *http.Client
	url    string
}

func NewHTTPRunner(url string) *HTTPRunner {
	t := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
	}
	
	return &HTTPRunner{
		client: &http.Client{Transport: t, Timeout: 10 * time.Second},
		url: url,
	}
}

func (r *HTTPRunner) DoRequest() (status int, latency time.Duration, err error) {
	start := time.Now()

	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return 0, 0, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	latency = time.Since(start)

	return resp.StatusCode, latency, nil
}
