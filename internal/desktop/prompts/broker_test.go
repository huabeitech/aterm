package prompts

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPromptAnswersAreScopedValidatedAndSingleUse(t *testing.T) {
	emitted := make(chan any, 8)
	broker := New(func(event any) { emitted <- event })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan []string, 1)
	go func() {
		answers, _ := broker.Ask(ctx, Prompt{SessionID: "one", Kind: "credentials", Fields: []Field{{Label: "Password", Secret: true}}})
		result <- answers
	}()
	prompt := (<-emitted).(map[string]any)["prompt"].(Prompt)
	if err := broker.Answer(prompt.ID, []string{}); err == nil {
		t.Fatal("wrong field count accepted")
	}
	if err := broker.Answer("other", []string{"secret"}); err == nil {
		t.Fatal("unknown prompt accepted")
	}
	if err := broker.Answer(prompt.ID, []string{"one-time"}); err != nil {
		t.Fatal(err)
	}
	if answers := <-result; len(answers) != 1 || answers[0] != "one-time" {
		t.Fatalf("answers: %v", answers)
	}
	if err := broker.Answer(prompt.ID, []string{"again"}); err == nil {
		t.Fatal("prompt answer was reused")
	}
	closed := (<-emitted).(map[string]any)
	if closed["type"] != "prompt-closed" || closed["id"] != prompt.ID {
		t.Fatal("prompt close event missing")
	}
}
func TestPromptCancellationClosesOutstandingModal(t *testing.T) {
	emitted := make(chan any, 8)
	broker := New(func(event any) { emitted <- event })
	done := make(chan error, 1)
	go func() {
		_, err := broker.Ask(context.Background(), Prompt{SessionID: "one", Kind: "host-key"})
		done <- err
	}()
	<-emitted
	broker.CancelSession("one")
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if (<-emitted).(map[string]any)["type"] != "prompt-closed" {
		t.Fatal("cancel left prompt open")
	}
	broker.Close()
	if _, err := broker.Ask(context.Background(), Prompt{}); !errors.Is(err, context.Canceled) {
		t.Fatal("closed broker accepted prompt")
	}
}
func TestEmptyApprovedHostKeyAnswerDiffersFromCancel(t *testing.T) {
	emitted := make(chan any, 4)
	broker := New(func(event any) { emitted <- event })
	done := make(chan error, 1)
	go func() {
		answers, err := broker.Ask(context.Background(), Prompt{Kind: "host-key"})
		if answers == nil && err == nil {
			err = errors.New("approval became cancellation")
		}
		done <- err
	}()
	prompt := (<-emitted).(map[string]any)["prompt"].(Prompt)
	if err := broker.Answer(prompt.ID, []string{}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
