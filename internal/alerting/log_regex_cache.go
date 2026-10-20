package alerting

import (
	"regexp"
	"sync"
)

// maxCachedLogRegexes bounds the compiled-pattern cache; user patterns are capped at maxLogRegexLen but not in number.
const maxCachedLogRegexes = 256

// logRegexCache holds compiled log rule patterns so each evaluation skips regexp.Compile.
type logRegexCache struct {
	mu sync.Mutex
	m  map[string]*regexp.Regexp
}

var logRegexes = &logRegexCache{m: make(map[string]*regexp.Regexp)}

// compileLogRegex returns the compiled pattern, caching successful compiles. Invalid patterns are not cached.
func compileLogRegex(pattern string) (*regexp.Regexp, error) {
	return logRegexes.get(pattern)
}

func (c *logRegexCache) get(pattern string) (*regexp.Regexp, error) {
	c.mu.Lock()
	re, ok := c.m[pattern]
	c.mu.Unlock()
	if ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxCachedLogRegexes {
		for k := range c.m {
			delete(c.m, k)
			break
		}
	}
	c.m[pattern] = re
	return re, nil
}
