package sourcehealth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"event-hunter/backend/internal/contexts/investigation/application/overview"
)

type HTTPProbe struct {
	name     overview.SourceName
	endpoint string
	client   *http.Client
	timeout  time.Duration
}

func NewHTTPProbe(name overview.SourceName, endpoint string, client *http.Client, timeout time.Duration) *HTTPProbe {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPProbe{name: name, endpoint: endpoint, client: client, timeout: timeout}
}

func (probe *HTTPProbe) Name() overview.SourceName {
	return probe.name
}

func (probe *HTTPProbe) Check(ctx context.Context) error {
	probeContext, cancel := context.WithTimeout(ctx, probe.timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(probeContext, http.MethodGet, probe.endpoint, nil)
	if err != nil {
		return err
	}
	response, err := probe.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("source returned %s", response.Status)
	}
	return nil
}

var _ overview.SourceProbe = (*HTTPProbe)(nil)
