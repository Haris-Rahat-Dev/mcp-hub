package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	target := flag.String("target", "", "hub endpoint URL (e.g. http://127.0.0.1:4117/mcp/context7)")
	apiKey := flag.String("api-key", "", "optional bearer token for hub auth")
	flag.Parse()

	if strings.TrimSpace(*target) == "" {
		fmt.Fprintln(os.Stderr, "--target is required")
		os.Exit(2)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	var mu sync.Mutex
	sessionID := ""

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go streamNotifications(ctx, client, *target, *apiKey, &mu, &sessionID)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		respBody, err := sendRequest(client, *target, *apiKey, line, &mu, &sessionID)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			continue
		}
		if len(respBody) == 0 {
			continue
		}
		if _, err := os.Stdout.Write(respBody); err != nil {
			return
		}
		if _, err := os.Stdout.Write([]byte("\n")); err != nil {
			return
		}
	}
}

func sendRequest(client *http.Client, target, apiKey, payload string, mu *sync.Mutex, sessionID *string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	mu.Lock()
	if *sessionID != "" {
		req.Header.Set("Mcp-Session-Id", *sessionID)
	}
	mu.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		mu.Lock()
		*sessionID = sid
		mu.Unlock()
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("bridge POST failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if resp.StatusCode == http.StatusAccepted {
		return nil, nil
	}
	return io.ReadAll(resp.Body)
}

func streamNotifications(ctx context.Context, client *http.Client, target, apiKey string, mu *sync.Mutex, sessionID *string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		mu.Lock()
		if *sessionID != "" {
			req.Header.Set("Mcp-Session-Id", *sessionID)
		}
		mu.Unlock()

		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
			mu.Lock()
			*sessionID = sid
			mu.Unlock()
		}
		if resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			time.Sleep(1 * time.Second)
			continue
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 4<<20)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := strings.TrimPrefix(line, "data: ")
			if !json.Valid([]byte(payload)) {
				continue
			}
			_, _ = os.Stdout.Write([]byte(payload))
			_, _ = os.Stdout.Write([]byte("\n"))
		}
		_ = resp.Body.Close()
		time.Sleep(250 * time.Millisecond)
	}
}
