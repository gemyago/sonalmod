package summarize

import (
	"context"
	"strings"
)

// Summarizer summarizes text into a concise form.
// This is a general-purpose abstraction, reusable beyond session titles.
type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
}

const defaultTruncatingSummarizerMaxLen = 50

// TruncatingSummarizerOption configures [NewTruncatingSummarizer].
type TruncatingSummarizerOption func(*TruncatingSummarizer)

// WithTruncatingSummarizerMaxLen sets the maximum byte length before truncation.
// Zero or negative values are treated as the default (50 bytes) after all options apply.
func WithTruncatingSummarizerMaxLen(n int) TruncatingSummarizerOption {
	return func(t *TruncatingSummarizer) {
		t.maxLen = n
	}
}

// TruncatingSummarizer shortens text for summaries (see [NewTruncatingSummarizer]).
type TruncatingSummarizer struct {
	maxLen int
}

// NewTruncatingSummarizer returns a summarizer that shortens text using optional functional opts.
// With no opts, text is shortened to at most 50 bytes at the last ASCII space within the first
// 50 bytes, or after 50 bytes when no space appears; an ellipsis is appended when shortened.
func NewTruncatingSummarizer(opts ...TruncatingSummarizerOption) *TruncatingSummarizer {
	t := &TruncatingSummarizer{}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	if t.maxLen <= 0 {
		t.maxLen = defaultTruncatingSummarizerMaxLen
	}
	return t
}

func (t *TruncatingSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	_ = ctx
	maxLen := t.maxLen
	if maxLen <= 0 {
		maxLen = defaultTruncatingSummarizerMaxLen
	}
	if len(text) <= maxLen {
		return text, nil
	}
	prefix := text[:maxLen]
	lastSpace := strings.LastIndexByte(prefix, ' ')
	if lastSpace <= 0 {
		return prefix + "...", nil
	}
	return text[:lastSpace] + "...", nil
}
