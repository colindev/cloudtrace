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
	return r.URL.String()
}

type Span struct {
	*trace.Span
}

func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	ctx, span := trace.StartSpan(ctx, name)
	return ctx, &Span{span}
}

func ApplyConfig(probability float64) {
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
}

func BuildTraceRoundTripper(project string, tp http.RoundTripper) (http.RoundTripper, error) {
	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID: project,
	})
	if err != nil {
		return nil, err
	}

	trace.RegisterExporter(exporter)

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

var tags map[string]string

func WithGlobalTags(m map[string]string) {
	tags = m
}

func WrapHandler(handler http.Handler, isPub bool, isHealth func(*http.Request) bool) http.Handler {
	return &ochttp.Handler{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			span := trace.FromContext(r.Context())
			attrs := []trace.Attribute{
				trace.StringAttribute("project", projectId),
				trace.StringAttribute("hostname", hostname),
			}
			for k, v := range tags {
				attrs = append(attrs, trace.StringAttribute(k, v))
			}
			span.AddAttributes(attrs...)
			handler.ServeHTTP(w, r)
		}),
		Propagation:      &propagation.HTTPFormat{},
		FormatSpanName:   formatSpanName,
		IsPublicEndpoint: isPub,
		IsHealthEndpoint: isHealth,
	}
}

func ConfigureServer(s *http.Server, h http.Handler, isPub bool, isHealth func(*http.Request) bool) {
	s.Handler = WrapHandler(h, isPub, isHealth)
}
