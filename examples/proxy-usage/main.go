// Package main demonstrates using MAIA's OpenAI-compatible proxy.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	proxyURL = "http://localhost:8080/proxy/v1/chat/completions"
)

// ChatRequest represents an OpenAI chat completion request.
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatResponse represents an OpenAI chat completion response.
type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

// Choice represents a response choice.
type Choice struct {
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

func main() {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	namespace := "proxy-example"

	fmt.Println("=== MAIA OpenAI Proxy Example ===")

	// Example 1: Store some preferences first
	fmt.Println("Step 1: Storing user preferences via MAIA API...")
	storePreferences(namespace)

	// Example 2: Chat with context injection
	fmt.Println("Step 2: Chatting through proxy (context will be injected)...")
	response := chat(apiKey, namespace, "What do you know about my preferences?", false)
	fmt.Printf("Assistant: %s\n\n", response)

	// Example 3: Store a memory through conversation
	fmt.Println("Step 3: Storing new memory through conversation...")
	response = chat(apiKey, namespace, "Remember that I'm working on a Python project called data-pipeline", false)
	fmt.Printf("Assistant: %s\n\n", response)

	// Example 4: Verify the memory was stored
	fmt.Println("Step 4: Verifying memory was stored...")
	response = chat(apiKey, namespace, "What project am I working on?", false)
	fmt.Printf("Assistant: %s\n\n", response)

	// Example 5: Streaming response
	fmt.Println("Step 5: Streaming response...")
	streamChat(apiKey, namespace, "Tell me a short joke about programming")

	// Example 6: Skip memory injection
	fmt.Println("\nStep 6: Chat without memory injection (fresh context)...")
	response = chatWithOptions(apiKey, namespace, "What do you know about me?", map[string]string{
		"X-MAIA-Skip-Memory": "true",
	})
	fmt.Printf("Assistant (no context): %s\n\n", response)

	fmt.Println("Done!")
}

// storePreferences uses the MAIA API directly to store some initial memories.
func storePreferences(namespace string) {
	memories := []map[string]interface{}{
		{
			"namespace": namespace,
			"content":   "User prefers dark mode in all applications",
			"type":      "semantic",
			"tags":      []string{"preference", "ui"},
		},
		{
			"namespace": namespace,
			"content":   "User's preferred programming language is Go",
			"type":      "semantic",
			"tags":      []string{"preference", "language"},
		},
		{
			"namespace": namespace,
			"content":   "User works in the America/New_York timezone",
			"type":      "semantic",
			"tags":      []string{"preference", "timezone"},
		},
	}

	for _, mem := range memories {
		data, _ := json.Marshal(mem)
		resp, err := http.Post(
			"http://localhost:8080/v1/memories",
			"application/json",
			bytes.NewReader(data),
		)
		if err != nil {
			log.Printf("Failed to store memory: %v", err)
			continue
		}
		resp.Body.Close()
		fmt.Printf("  Stored: %s\n", mem["content"])
	}
	fmt.Println()
}

// chat sends a chat request through the MAIA proxy.
func chat(apiKey, namespace, message string, stream bool) string {
	return chatWithOptions(apiKey, namespace, message, map[string]string{})
}

// chatWithOptions sends a chat request with custom headers.
func chatWithOptions(apiKey, namespace, message string, extraHeaders map[string]string) string {
	req := ChatRequest{
		Model: "gpt-4o-mini", // Use a cheaper model for demo
		Messages: []Message{
			{Role: "system", Content: "You are a helpful assistant with memory capabilities."},
			{Role: "user", Content: message},
		},
	}

	data, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", proxyURL, bytes.NewReader(data))

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("X-MAIA-Namespace", namespace)

	for k, v := range extraHeaders {
		httpReq.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("Request failed: %v", err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Error response (%d): %s", resp.StatusCode, string(body))
		return ""
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		log.Printf("Failed to decode response: %v", err)
		return ""
	}

	if len(chatResp.Choices) == 0 {
		return ""
	}

	return chatResp.Choices[0].Message.Content
}

// streamChat demonstrates streaming responses through the proxy.
func streamChat(apiKey, namespace, message string) {
	req := ChatRequest{
		Model: "gpt-4o-mini",
		Messages: []Message{
			{Role: "user", Content: message},
		},
		Stream: true,
	}

	data, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest("POST", proxyURL, bytes.NewReader(data))

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("X-MAIA-Namespace", namespace)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("Request failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Error response (%d): %s", resp.StatusCode, string(body))
		return
	}

	fmt.Print("Assistant: ")

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// SSE format: "data: {...}"
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
	}

	fmt.Println()
}
