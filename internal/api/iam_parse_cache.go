package api

import "sync"

const (
	parsedDocumentCacheMax   = 2048
	parsedDocumentCacheBytes = 4 << 20
	parsedDocumentMaxKey     = 64 << 10
)

var parsedDocuments = struct {
	sync.RWMutex
	m     map[string]*Document
	bytes int
}{m: map[string]*Document{}}

// parseDocumentCached memoizes valid documents by their exact text, so a
// policy is parsed once instead of once per resource a list authorizes. A
// changed document is a different key. Callers must treat the result as read only.
func parseDocumentCached(raw string) (*Document, error) {
	parsedDocuments.RLock()
	doc, ok := parsedDocuments.m[raw]
	parsedDocuments.RUnlock()
	if ok {
		return doc, nil
	}
	doc, err := ParseDocument(raw)
	if err != nil {
		return nil, err
	}
	if len(raw) > parsedDocumentMaxKey {
		return doc, nil
	}
	parsedDocuments.Lock()
	if len(parsedDocuments.m) >= parsedDocumentCacheMax || parsedDocuments.bytes+len(raw) > parsedDocumentCacheBytes {
		clear(parsedDocuments.m)
		parsedDocuments.bytes = 0
	}
	parsedDocuments.m[raw] = doc
	parsedDocuments.bytes += len(raw)
	parsedDocuments.Unlock()
	return doc, nil
}
