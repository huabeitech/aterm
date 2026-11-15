package prompts

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"sync"
)

type Field struct {
	Label  string `json:"label"`
	Secret bool   `json:"secret"`
}
type Prompt struct {
	ID        string  `json:"id"`
	SessionID string  `json:"sessionID"`
	Title     string  `json:"title"`
	Message   string  `json:"message"`
	Kind      string  `json:"kind"`
	Fields    []Field `json:"fields"`
}
type pending struct {
	prompt   Prompt
	response chan []string
}
type Broker struct {
	mu      sync.Mutex
	pending map[string]*pending
	emit    func(any)
	closed  bool
}

func New(emit func(any)) *Broker { return &Broker{pending: map[string]*pending{}, emit: emit} }
func (b *Broker) Ask(ctx context.Context, prompt Prompt) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	prompt.ID = uuid.NewString()
	if prompt.Fields == nil {
		prompt.Fields = []Field{}
	}
	request := &pending{prompt: prompt, response: make(chan []string, 1)}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, context.Canceled
	}
	b.pending[prompt.ID] = request
	b.mu.Unlock()
	if b.emit != nil {
		b.emit(map[string]any{"type": "prompt", "prompt": prompt})
	}
	defer func() {
		b.mu.Lock()
		delete(b.pending, prompt.ID)
		b.mu.Unlock()
		if b.emit != nil {
			b.emit(map[string]any{"type": "prompt-closed", "id": prompt.ID})
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case answers := <-request.response:
		if answers == nil {
			return nil, context.Canceled
		}
		return answers, nil
	}
}
func (b *Broker) Answer(id string, answers []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	request := b.pending[id]
	if request == nil {
		return errors.New("this authentication prompt has already closed")
	}
	if answers != nil {
		if len(answers) != len(request.prompt.Fields) {
			return errors.New("authentication response has the wrong number of fields")
		}
		for _, answer := range answers {
			if len(answer) > 4096 {
				return errors.New("authentication response is too long")
			}
		}
		answers = append([]string{}, answers...)
	}
	delete(b.pending, id)
	request.response <- answers
	return nil
}
func (b *Broker) CancelSession(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, request := range b.pending {
		if request.prompt.SessionID == sessionID {
			delete(b.pending, id)
			request.response <- nil
		}
	}
}
func (b *Broker) Close() {
	b.mu.Lock()
	b.closed = true
	for id, request := range b.pending {
		delete(b.pending, id)
		request.response <- nil
	}
	b.mu.Unlock()
}
