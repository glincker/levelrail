package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// fakePRComments is the in-memory comment thread every provider fake shares.
type fakePRComments struct {
	nextID    int64
	comments  []gitprovider.Comment
	updates   []gitprovider.Comment
	listErr   error
	updateErr error
	listCalls int
}

func (f *fakePRComments) create(body string) int64 {
	f.nextID++
	f.comments = append(f.comments, gitprovider.Comment{ID: f.nextID, Body: body})
	return f.nextID
}

func (f *fakePRComments) list() ([]gitprovider.Comment, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]gitprovider.Comment(nil), f.comments...), nil
}

func (f *fakePRComments) update(id int64, body string) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	for i := range f.comments {
		if f.comments[i].ID == id {
			f.comments[i].Body = body
			f.updates = append(f.updates, gitprovider.Comment{ID: id, Body: body})
			return nil
		}
	}
	return &gitprovider.APIError{Prefix: "fake", API: "fake", StatusCode: http.StatusNotFound}
}
