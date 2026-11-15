package ssh

import (
	"context"
	cryptossh "golang.org/x/crypto/ssh"
)

type SessionFactory interface {
	NewSession() (*cryptossh.Session, error)
}

// Bound outstanding opens when a server ignores channel requests. A late
// session belongs to the opener and is closed if its caller has canceled.
var openSlots = make(chan struct{}, 16)

func OpenSession(ctx context.Context, client SessionFactory) (*cryptossh.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case openSlots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	type opened struct {
		session *cryptossh.Session
		err     error
	}
	reply := make(chan opened)
	go func() {
		defer func() { <-openSlots }()
		session, err := client.NewSession()
		select {
		case reply <- opened{session, err}:
		case <-ctx.Done():
			if session != nil {
				session.Close()
			}
		}
	}()
	select {
	case value := <-reply:
		return value.session, value.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
