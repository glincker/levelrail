package api

import "sync"

const parsedDocumentCacheMax = 2048

var parsedDocuments = struct {
	sync.RWMutex
	m map[string]*Document
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
	parsedDocuments.Lock()
	if len(parsedDocuments.m) >= parsedDocumentCacheMax {
		clear(parsedDocuments.m)
	}
	parsedDocuments.m[raw] = doc
	parsedDocuments.Unlock()
	return doc, nil
}
