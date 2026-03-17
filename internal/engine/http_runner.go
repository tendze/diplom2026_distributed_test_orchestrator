package engine

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/valyala/fasthttp"
)

const (
	RequestTimeout = 4 * time.Second
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
		MaxIdleConns:        10000,
		MaxIdleConnsPerHost: 10000,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
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

// FastHTTPRunner — структура для выполнения HTTP-запросов через пул соединений с помощью fasthttp.
type FastHTTPRunner struct {
	client *fasthttp.Client
	url    string
}

func NewFastHTTPRunner(url string) *FastHTTPRunner {
	// Настраиваем клиент fasthttp для максимальной производительности
	client := &fasthttp.Client{
		Name:                     "Distributed-Load-Generator",
		MaxConnsPerHost:          20000,
		ReadTimeout:              5 * time.Second,
		WriteTimeout:             5 * time.Second,
		MaxIdleConnDuration:      60 * time.Second,
		NoDefaultUserAgentHeader: true,
		MaxIdemponentCallAttempts: 1, // Чтобы не ретраить при сетевых ошибках
	}
	// Настройка fasthttp
	return &FastHTTPRunner{
		client: client,
		url:    url,
	}
}

func (r *FastHTTPRunner) DoRequest(ctx context.Context) (status int, latency time.Duration, size uint64, err error) {
	start := time.Now()

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(r.url)
	req.Header.SetMethod(fasthttp.MethodGet)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(RequestTimeout)
	}

	err = r.client.DoDeadline(req, resp, deadline)
	latency = time.Since(start)

	if err != nil {
		return 0, 0, 0, err
	}

	status = resp.StatusCode()
	size = uint64(len(resp.Body()))

	return status, latency, size, nil
}
