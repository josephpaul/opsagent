package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"
)

// QueryRequest is the JSON body sent to the AI agent /query endpoint.
type QueryRequest struct {
	Question string `json:"question"`
	Provider string `json:"provider,omitempty"` // gemini | anthropic | openai
	Model    string `json:"model,omitempty"`     // optional model override
}

// QueryResponse is the JSON response from the AI agent /query endpoint.
type QueryResponse struct {
	Diagnosis string `json:"diagnosis"`
	Details   string `json:"details"`
}

func runQuery(cmd *cobra.Command, args []string) error {
	question := args[0]
	url := aiAgentURL + "/query"

	reqBody := QueryRequest{
		Question: question,
		Provider: provider,
		Model:    model,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request to AI agent failed (is it running at %s?): %w", aiAgentURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AI agent returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var queryResp QueryResponse
	if err := json.Unmarshal(respBody, &queryResp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	// Print diagnosis in a readable format.
	fmt.Println("--- Diagnosis ---")
	fmt.Println(queryResp.Diagnosis)
	if queryResp.Details != "" {
		fmt.Println()
		fmt.Println("--- Details ---")
		fmt.Println(queryResp.Details)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}
