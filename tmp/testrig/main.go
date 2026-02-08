package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	btclient "github.com/braintrustdata/braintrust-go"
	"github.com/eolymp/go-agent/braintrust"
)

func main() {
	// Configuration
	apiKey := "sk-x04uu4NE1l4zcJVC9XEAtq1JNpWyiymAwCjfcK5wlmLRssam"
	projectID := "e35b70f9-9e67-4460-bbd3-9d665d90f023"
	slug := "gds-expert-g1d3"

	// Set the API key as environment variable (braintrust client uses this)
	os.Setenv("BRAINTRUST_API_KEY", apiKey)

	// Create braintrust client
	client := btclient.NewClient()

	// Create prompter
	prompter := braintrust.NewPrompter(client, projectID)

	// Load prompt
	ctx := context.Background()
	prompt, err := prompter.Load(ctx, slug)
	if err != nil {
		log.Fatalf("Failed to load prompt: %v", err)
	}

	// Print as JSON
	jsonData, err := json.MarshalIndent(prompt, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal prompt to JSON: %v", err)
	}

	fmt.Println(string(jsonData))
}
