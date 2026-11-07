package scheduledtask

import (
	"sync/atomic"
	"time"
)

// Settings key and env seed for the default request timeout of an http job.
const (
	KeyHTTPTimeoutSeconds = "jobs.http_timeout_seconds"
	EnvHTTPTimeoutSeconds = "APP_JOBS_HTTP_TIMEOUT_SECONDS"
)

var httpTimeoutResolver atomic.Pointer[func() time.Duration]

// SetHTTPTimeoutResolver feeds the saved runtime setting so changes apply to
// the next run without a restart.
func SetHTTPTimeoutResolver(fn func() time.Duration) {
	httpTimeoutResolver.Store(&fn)
}

func runtimeHTTPTimeout() time.Duration {
	if fn := httpTimeoutResolver.Load(); fn != nil {
		return (*fn)()
	}
	return 0
}
