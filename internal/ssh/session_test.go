package ssh

import (
	"context"
	"errors"
	cryptossh "golang.org/x/crypto/ssh"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type blockedSessionFactory struct {
	started atomic.Int32
	release chan struct{}
}

func (f *blockedSessionFactory) NewSession() (*cryptossh.Session, error) {
	f.started.Add(1)
	<-f.release
	return nil, errors.New("fixture released")
}
func TestCanceledChannelOpensAreBoundedAndReclaimCapacity(t *testing.T) {
	factory := &blockedSessionFactory{release: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := OpenSession(ctx, factory); !errors.Is(err, context.DeadlineExceeded) {
				t.Error("channel open ignored cancellation", err)
			}
		}()
	}
	group.Wait()
	if count := factory.started.Load(); count > 16 {
		t.Fatalf("unbounded pending channel opens: %d", count)
	}
	close(factory.release)
	next, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, err := OpenSession(next, factory); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("canceled channel opens retained capacity", err)
	}
}
