package cloudtrace

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
)

var (
	projectId string
	hostname  string
)

func init() {
	hostname, _ = os.Hostname()
	r, _ := http.NewRequest(
		http.MethodGet,
		"http://metadata.google.internal/computeMetadata/v1/project/project-id",
		nil)
	r.Header.Set("Metadata-Flavor", "Google")
	c := http.Client{
		Timeout: time.Millisecond * 100,
	}
	res, err := c.Do(r)
	if err != nil {
		log.Println("skip set project:", err)
		return
	}
	defer res.Body.Close()
	b, _ := ioutil.ReadAll(res.Body)
	projectId = strings.TrimSpace(string(b))
}

func formatSpanName(r *http.Request) string {
	return r.URL.Scheme + "://" + r.URL.Host + "/" + r.URL.Path
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
		Propagation:    &propagation.HTTPFormat{},
		FormatSpanName: formatSpanName,
	}, nil
}

func WithRouteTag(handler http.Handler, route string) http.Handler {
	return ochttp.WithRouteTag(handler, route)
}

func ConfigureServer(s *http.Server, h http.Handler, isPub bool, isHealth func(*http.Request) bool) {
	s.Handler = &ochttp.Handler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := trace.FromContext(r.Context())
			span.AddAttributes(trace.StringAttribute("project", projectId))
			span.AddAttributes(trace.StringAttribute("hostname", hostname))
			h.ServeHTTP(w, r)
		}),
		Propagation:      &propagation.HTTPFormat{},
		FormatSpanName:   formatSpanName,
		IsPublicEndpoint: isPub,
		IsHealthEndpoint: isHealth,
	}
}
