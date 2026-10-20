package dockerssh

import (
	"context"
	"io"
)

// Saver streams an image from the source host through the socket tunnel,
// the read-only replacement for running a save command there.
type Saver struct {
	Cfg Config
}

// Save opens a session, requests the image archive and returns a stream
// whose Close also ends the SSH connection.
func (s Saver) Save(ctx context.Context, ref string) (io.ReadCloser, error) {
	sess, err := Open(ctx, s.Cfg)
	if err != nil {
		return nil, err
	}
	rc, err := sess.ImageSave(ctx, ref)
	if err != nil {
		sess.Close()
		return nil, err
	}
	return &sessionStream{ReadCloser: rc, sess: sess}, nil
}

type sessionStream struct {
	io.ReadCloser
	sess *Session
}

func (s *sessionStream) Close() error {
	err := s.ReadCloser.Close()
	s.sess.Close()
	return err
}
