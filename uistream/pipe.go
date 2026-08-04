package uistream

import (
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"

	"github.com/open-ai-sdk/ai-go/aikit"
)

// Pipe drains events through p. It always calls Finish exactly once after a
// successful encoder construction, including failures in Start or Encode.
func Pipe(ctx context.Context, w io.Writer, events iter.Seq2[aikit.StepEvent, error], p Protocol, opts Options) (retErr error) {
	if p.NewEncoder == nil || p.Framer == nil {
		return fmt.Errorf("uistream: incomplete protocol")
	}
	var e Encoder
	var terminal error
	var writeFailed bool
	write := func(frames []Frame) bool {
		for _, frame := range frames {
			if err := p.Framer.WriteFrame(w, frame); err != nil {
				if !writeFailed && opts.OnWriteError != nil {
					opts.OnWriteError(err)
				}
				writeFailed = true
				terminal = err
				return false
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		return true
	}
	defer func() {
		if v := recover(); v != nil {
			terminal = fmt.Errorf("panic: %v", v)
		}
		if e != nil {
			var frames []Frame
			func() {
				defer func() {
					if v := recover(); v != nil && terminal == nil {
						terminal = fmt.Errorf("panic in finish: %v", v)
					}
				}()
				var err error
				frames, err = e.Finish(terminal)
				if err != nil && terminal == nil {
					terminal = err
				}
			}()
			if !writeFailed {
				write(frames)
			}
		}
		retErr = terminal
	}()
	e = p.NewEncoder(opts)
	if e == nil {
		return fmt.Errorf("uistream: nil encoder")
	}
	frames, err := e.Start()
	if err != nil {
		terminal = err
		return nil
	}
	if !write(frames) {
		return nil
	}
	if events == nil {
		return nil
	}
	for ev, err := range events {
		if ctx.Err() != nil {
			terminal = ctx.Err()
			return nil
		}
		if err != nil {
			terminal = err
			return nil
		}
		if ev.Type == aikit.StepEventError {
			terminal = ev.Error
			if terminal == nil {
				terminal = fmt.Errorf("uistream: error event without error")
			}
			return nil
		}
		frames, err = e.Encode(ev)
		if err != nil {
			terminal = err
			return nil
		}
		if !write(frames) {
			return nil
		}
	}
	return nil
}
