// Package spinner for terminal/console "loading" indicator
package spinner

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"m/pkg/llm"
)

type Spinner struct {
	stopChan chan struct{}
	once     sync.Once
}

func (s *Spinner) Fake() bool {
	return s.stopChan == nil
}

func (s *Spinner) Reset() {
	s.quit()
	<-s.stopChan
}

var (
	Out    = os.Stderr
	NoSpin = false
)

// Start kicks off the spinner in a background goroutine, writing to Out
func Start(ctx context.Context) *Spinner {
	if NoSpin || Out == nil || Out == io.Discard {
		return &Spinner{stopChan: nil}
	}
	s := &Spinner{
		stopChan: make(chan struct{}),
	}
	out := Out
	go func() {
		frames := []string{"|", "/", "-", "\\"}
		i := 0
		defer func() {
			_, _ = fmt.Fprint(out, "\r \r")
		}()
		for {
			select {
			case <-ctx.Done():
				s.quit()
				return
			case <-s.stopChan:
				// When we receive a signal on this channel, stop the loop
				return
			default:
				_, _ = fmt.Fprintf(out, "\r%s", frames[i])
				i = (i + 1) % len(frames)
				for range 4 {
					time.Sleep(50 * time.Millisecond)
					if ctx.Err() != nil {
						break
					}
				}
			}
		}
	}()

	return s
}

// HandleChatResponse implements [llm.ChatStreamHandlerFunc] (for cascading chat handlers) so our spinner stops when the first token is generated.
func (s *Spinner) HandleChatResponse(_ llm.ChatResponse) error {
	s.quit()
	return nil
}

func (s *Spinner) quit() {
	s.once.Do(func() {
		if s.Fake() {
			return
		}
		close(s.stopChan) // Signal the goroutine to stop
		time.Sleep(200 * time.Millisecond)
	})
}
