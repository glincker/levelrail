package api

import (
	"encoding/json"
	"net/http"
)

// maxDecodeBodyBytes matches the largest per-handler cap so adopting decodeReq never rejects a body an old handler accepted.
const maxDecodeBodyBytes int64 = 8 << 20

const msgInvalidRequestBody = "invalid request body"

// decodeReq reads a JSON request body into dst. On failure it writes the
// standard 400 and returns false. Unknown fields are accepted on purpose so older clients keep working.
// Handlers keep `var req T` and pass &req: the body-ref authz guard test finds decodes by that shape.
func decodeReq[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	return decodeReqLimit(w, r, dst, maxDecodeBodyBytes)
}

// decodeReqLimit is decodeReq with a tighter body cap.
func decodeReqLimit[T any](w http.ResponseWriter, r *http.Request, dst *T, limit int64) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidRequestBody)
		return false
	}
	return true
}
