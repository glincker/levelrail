package secretmask

import "io"

// Writer masks a byte stream. It holds back the last maxLen-1 bytes of each
// write so a secret split across two writes is still seen whole.
type Writer struct {
	dst   io.Writer
	reg   *Registry
	carry string
}

// NewWriter masks with the process-wide registry.
func NewWriter(dst io.Writer) *Writer { return &Writer{dst: dst, reg: shared} }

// NewWriterFor masks with reg.
func NewWriterFor(dst io.Writer, reg *Registry) *Writer { return &Writer{dst: dst, reg: reg} }

// Write always reports len(p) so io.Copy keeps draining even when dst fails late.
// The carry stays raw: masking it early would hide a longer secret that a shorter one sits inside.
func (w *Writer) Write(p []byte) (int, error) {
	buf := w.carry + string(p)
	// An empty registry has maxLen 0: hold nothing so output streams through.
	hold := max(0, w.reg.maxLen()-1)
	if hold > len(buf) {
		hold = len(buf)
	}
	cut := len(buf) - hold
	for _, sp := range w.reg.mergedSpans(buf) {
		if sp[0] < cut && cut < sp[1] {
			cut = sp[0]
		}
	}
	w.carry = buf[cut:]
	if cut > 0 {
		if _, err := io.WriteString(w.dst, w.reg.Mask(buf[:cut])); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// Flush emits the held-back tail. Call it when the stream ends.
func (w *Writer) Flush() error {
	tail := w.reg.Mask(w.carry)
	w.carry = ""
	if tail == "" {
		return nil
	}
	_, err := io.WriteString(w.dst, tail)
	return err
}

// Close flushes; it does not close dst.
func (w *Writer) Close() error { return w.Flush() }
