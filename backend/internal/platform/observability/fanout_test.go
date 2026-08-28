package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestFanoutHandlerWritesEveryChild(t *testing.T) {
	var first bytes.Buffer
	var second bytes.Buffer
	logger := slog.New(NewFanoutHandler(
		slog.NewTextHandler(&first, nil),
		slog.NewJSONHandler(&second, nil),
	))
	logger.InfoContext(context.Background(), "processed event", "correlation_id", "ORDER-1")

	if !strings.Contains(first.String(), "correlation_id=ORDER-1") {
		t.Fatalf("text handler did not receive record: %s", first.String())
	}
	if !strings.Contains(second.String(), `"correlation_id":"ORDER-1"`) {
		t.Fatalf("JSON handler did not receive record: %s", second.String())
	}
}
