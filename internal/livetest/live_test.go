//go:build live

package livetest

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thomas-huang/caosi/internal/app"
	"github.com/thomas-huang/caosi/internal/config"
)

const (
	attemptTimeout = 2 * time.Minute
	maxAttempts    = 4
)

var retryWait = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second}

var listenRe = regexp.MustCompile(`正在监听 http://(\S+)`)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestLiveConversionContract(t *testing.T) {
	dir, err := liveConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	file, err := loadLiveFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	rep := newRecorder(dir, file)
	if miss := missingUpstream(file); len(miss) > 0 {
		t.Logf("no Provider for %s (continue)", joinProtocols(miss))
	}
	if names := placeholderNames(file); len(names) > 0 {
		t.Logf("skip placeholder api_key: %s", strings.Join(names, ", "))
	}
	t.Cleanup(func() {
		fmt.Fprint(os.Stdout, "\n"+rep.markdown())
		mdPath, htmlPath, err := rep.write()
		if err != nil {
			t.Logf("write report: %v", err)
			return
		}
		t.Logf("report %s\n  %s", mdPath, htmlPath)
	})

	base := startCaosi(t, dir)

	for _, name := range usableNames(file) {
		p := file.Providers[name]
		t.Run(name, func(t *testing.T) {
			dead := false
			for _, client := range clientProtocols {
				for _, stream := range []bool{false, true} {
					mode := "json"
					if stream {
						mode = "stream"
					}
					t.Run(string(client)+"/"+mode, func(t *testing.T) {
						ev := event{Provider: name, Upstream: p.Protocol, Client: client, Stream: stream}
						if dead {
							ev.Outcome = outcomeSkip
							ev.Err = "provider already 401/403"
							rep.add(ev)
							t.Skip("provider already 401/403")
						}
						cells := bundleCells(client, p.Protocol)
						status, body, err := liveCall(base, name, client, stream, cells)
						if authFail(status) {
							dead = true
							ev.Outcome = outcomeFail
							ev.Err = fmt.Sprintf("status %d (provider credential): %s", status, snippet(body))
							rep.add(ev)
							t.Fatalf("%s", ev.Err)
						}
						if bundleErr := liveErr(client, stream, status, body, err); bundleErr != nil {
							ev.Outcome = outcomeFail
							ev.Err = bundleErr.Error()
							rep.add(ev)
							t.Errorf("bundle: %v", bundleErr)
							for _, c := range cells {
								t.Run(string(c), func(t *testing.T) {
									cev := event{Provider: name, Upstream: p.Protocol, Client: client, Stream: stream, Cell: string(c)}
									if dead {
										cev.Outcome = outcomeSkip
										cev.Err = "provider already 401/403"
										rep.add(cev)
										t.Skip("provider already 401/403")
									}
									st, b, e := liveCall(base, name, client, stream, []cell{c})
									if authFail(st) {
										dead = true
										cev.Outcome = outcomeFail
										cev.Err = fmt.Sprintf("status %d (provider credential): %s", st, snippet(b))
										rep.add(cev)
										t.Fatalf("%s", cev.Err)
									}
									if err := liveErr(client, stream, st, b, e); err != nil {
										cev.Outcome = outcomeFail
										cev.Err = err.Error()
										rep.add(cev)
										t.Fatal(err)
									}
									cev.Outcome = outcomePass
									rep.add(cev)
								})
							}
							return
						}
						ev.Outcome = outcomePass
						rep.add(ev)
					})
				}
			}
		})
	}
}

func authFail(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden
}

func liveErr(client config.Protocol, stream bool, status int, body []byte, err error) error {
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("status %d: %s", status, snippet(body))
	}
	if err := parseClientResponse(client, stream, body); err != nil {
		return fmt.Errorf("unparseable %s: %v\n%s", client, err, snippet(body))
	}
	return nil
}

func liveCall(base, provider string, client config.Protocol, stream bool, cells []cell) (int, []byte, error) {
	body, err := requestBody(client, stream, cells)
	if err != nil {
		return 0, nil, err
	}
	url := base + "/" + provider + clientPath(client, stream)
	var status int
	var respBody []byte
	for attempt := 0; attempt < maxAttempts; attempt++ {
		status, respBody, err = once(url, body)
		if err == nil && status != http.StatusTooManyRequests && status < 500 {
			return status, respBody, nil
		}
		if attempt == maxAttempts-1 {
			if err != nil {
				return status, respBody, fmt.Errorf("after %d attempts: %w", maxAttempts, err)
			}
			return status, respBody, nil
		}
		time.Sleep(retryWait[attempt])
	}
	return status, respBody, err
}

func once(url string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), attemptTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, b, err
	}
	return resp.StatusCode, b, nil
}

func startCaosi(t *testing.T, configDir string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stderr := &syncBuf{}
	done := make(chan int, 1)
	consumed := false
	go func() {
		done <- app.MainContext(ctx, []string{"caosi", "--config-dir", configDir, "--port", "0", "--log-level", "warn"}, io.Discard, stderr)
	}()
	t.Cleanup(func() {
		cancel()
		if consumed {
			return
		}
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Errorf("caosi did not shut down")
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	var base string
	for time.Now().Before(deadline) {
		select {
		case code := <-done:
			consumed = true
			t.Fatalf("caosi exited %d:\n%s", code, stderr.String())
		default:
		}
		if u := listenURL(stderr.String()); u != "" {
			base = u
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if base == "" {
		t.Fatalf("caosi not ready:\n%s", stderr.String())
	}
	waitHealth(t, base)
	return base
}

func listenURL(stderr string) string {
	m := listenRe.FindStringSubmatch(stderr)
	if len(m) != 2 {
		return ""
	}
	return "http://" + m[1]
}

func waitHealth(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last error
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(base + "/health")
		if err != nil {
			last = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return
		}
		last = fmt.Errorf("health %d", resp.StatusCode)
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("health: %v", last)
}

func snippet(body []byte) string {
	const n = 512
	s := string(bytes.TrimSpace(body))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
