package cloudtrace

import (
	"context"
	"net/http"
	"os"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
)

var (
	hostname string
)

func init() {
	hostname, _ = os.Hostname()
}

type Span struct {
	*trace.Span
}

func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	ctx, span := trace.StartSpan(ctx, name)
	return ctx, &Span{span}
}

func BuildTraceRoundTripper(project string, tp http.RoundTripper, probability float64) (http.RoundTripper, error) {
	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID: project,
	})
	if err != nil {
		return nil, err
	}

	trace.RegisterExporter(exporter)
	// trace.Config{DefaultSampler: trace.AlwaysSample()}
	trace.ApplyConfig(trace.Config{
		// DefaultSampler is the default sampler used when creating new spans.
		DefaultSampler: trace.ProbabilitySampler(probability),

		// IDGenerator is for internal use only.
		IDGenerator: nil,

		// MaxAnnotationEventsPerSpan is max number of annotation events per span
		MaxAnnotationEventsPerSpan: 0,

		// MaxMessageEventsPerSpan is max number of message events per span
		MaxMessageEventsPerSpan: 0,

		// MaxAnnotationEventsPerSpan is max number of attributes per span
		MaxAttributesPerSpan: 0,

		// MaxLinksPerSpan is max number of links per span
		MaxLinksPerSpan: 0,
	})

	return &ochttp.Transport{
		Base: tp,
		// Use Google Cloud propagation format.
		Propagation: &propagation.HTTPFormat{},
	}, nil
}

func ConfigureServer(s *http.Server, h http.Handler, isPub bool, isHealth func(*http.Request) bool) {
	s.Handler = &ochttp.Handler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := trace.FromContext(r.Context())
			span.AddAttributes(trace.StringAttribute("hostname", hostname))
			h.ServeHTTP(w, r)
		}),
		Propagation:      &propagation.HTTPFormat{},
		IsPublicEndpoint: isPub,
		IsHealthEndpoint: isHealth,
	}
}
