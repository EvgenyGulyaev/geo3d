package geo

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	maxRetries     = 3
	initialBackoff = 500 * time.Millisecond
)

// doWithRetry выполняет HTTP-запрос с повторами при сетевых ошибках, 429 и 5xx.
// Экспоненциальный backoff: 500ms → 1s → 2s.
func doWithRetry(client *http.Client, req *http.Request) (*http.Response, error) {
	var lastErr error
	backoff := initialBackoff

	// Сохраняем тело запроса для повторных попыток
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, lastErr = io.ReadAll(req.Body)
		if lastErr != nil {
			return nil, fmt.Errorf("read request body: %w", lastErr)
		}
		req.Body.Close()
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Восстанавливаем body для каждой попытки
		if bodyBytes != nil {
			req.Body = io.NopCloser(io.NewSectionReader(
				newBytesReaderAt(bodyBytes), 0, int64(len(bodyBytes)),
			))
			req.ContentLength = int64(len(bodyBytes))
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			slog.Warn("HTTP request failed",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", err)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		// Не ретраить клиентские ошибки (кроме 429)
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return resp, nil
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			slog.Warn("Retryable HTTP error",
				"attempt", attempt+1,
				"max_retries", maxRetries,
				"error", lastErr)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		// Не ретраим 4xx (кроме 429)
		return resp, nil
	}

	return nil, fmt.Errorf("all %d retry attempts failed: %w", maxRetries, lastErr)
}

// bytesReaderAt реализует io.ReaderAt для []byte.
type bytesReaderAt struct {
	data []byte
}

func newBytesReaderAt(data []byte) *bytesReaderAt {
	return &bytesReaderAt{data: data}
}

func (r *bytesReaderAt) ReadAt(p []byte, off int64) (n int, err error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n = copy(p, r.data[off:])
	if n < len(p) {
		err = io.EOF
	}
	return
}
