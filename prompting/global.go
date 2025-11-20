package prompting

import (
	"context"
	"os"
	"sync"

	"github.com/braintrustdata/braintrust-go"
)

var prompter *Prompter
var initialize sync.Once

func DefaultPrompter() *Prompter {
	initialize.Do(func() {
		prompter = NewPrompter(braintrust.NewClient(), os.Getenv("BRAINTRUST_PROJECT"))
	})

	return prompter
}

func SetDefaultPrompter(t *Prompter) {
	initialize.Do(func() {})
	prompter = t
}

func Load(ctx context.Context, slug string) (*Prompt, error) {
	return DefaultPrompter().Load(ctx, slug)
}
