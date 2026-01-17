package engine

import (
	"io"
	"net/http"
	"time"
)

// HTTPRunner инкапсулирует логику выполнения HTTP-запросов.
// Использует переиспользуемый http.Client с настроенным Transport
// для поддержки большого количества одновременных соединений (Connection Pooling).
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
		url:    url,
	}
}

// DoRequest выполняет одиночный HTTP GET запрос и возвращает метрики ответа.
func (r *HTTPRunner) DoRequest() (status int, latency time.Duration, size uint64, err error) {
	start := time.Now()

	req, err := http.NewRequest(http.MethodGet, r.url, nil)
	if err != nil {
		return 0, 0, 0, err
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()

	latency = time.Since(start)

	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return resp.StatusCode, 0, 0, err
	}

	return resp.StatusCode, latency, uint64(n), nil
}
