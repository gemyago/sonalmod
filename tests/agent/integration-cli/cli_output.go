package main

import (
	"context"
	"fmt"
	"io"
	"iter"

	"github.com/gemyago/sonalmod/runtime/agent"
)

// streamTextResult is implemented by *agent.RunResult for CLI streaming.
type streamTextResult interface {
	ConsumeEventsAsStringSeq(context.Context) iter.Seq2[string, error]
}

var _ streamTextResult = (*agent.RunResult)(nil)

func streamAgentOutput(ctx context.Context, output io.Writer, result streamTextResult) error {
	for chunk, err := range result.ConsumeEventsAsStringSeq(ctx) {
		if err != nil {
			return err
		}
		if _, werr := fmt.Fprint(output, chunk); werr != nil {
			return werr
		}
	}
	return nil
}
