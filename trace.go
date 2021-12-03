package cloudtrace

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/compute/metadata"
	"contrib.go.opencensus.io/exporter/stackdriver"
	"contrib.go.opencensus.io/exporter/stackdriver/propagation"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/trace"
	"golang.org/x/oauth2/google"
)

var (
	projectId string
	hostname  string
)

func init() {
	hostname, _ = os.Hostname()
	if metadata.OnGCE() {
		projectId, _ = metadata.ProjectID()
	} else {
		projectId = "unknown"
	}
	log.Println("set project:", projectId)
}

func Debug() {
	certs, err := google.FindDefaultCredentials(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(certs.ProjectID)
	fmt.Println(string(certs.JSON))
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

func ApplyConfig(project string, probability float64) error {
	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		ProjectID: project,
	})
	if err != nil {
		return err
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

	return nil
}

func BuildTraceRoundTripper(tp http.RoundTripper) http.RoundTripper {

	return &ochttp.Transport{
		Base: tp,
		// Use Google Cloud propagation format.
		Propagation:    &propagation.HTTPFormat{},
		FormatSpanName: formatSpanName,
	}
}

func WithRouteTag(handler http.Handler, route string) http.Handler {
	return ochttp.WithRouteTag(handler, route)
}

type Tags map[string]string

var tags = Tags{}

func (t Tags) Set(s string) error {
	ss := strings.SplitN(s, "=", 2)
	t[ss[0]] = ss[1]
	return nil
}

func (t Tags) String() string {
	ss := []string{}
	for k, v := range map[string]string(t) {
		ss = append(ss, k+"="+v)
	}

	return strings.Join(ss, ", ")
}

func WithGlobalTags(m Tags) {
	for k, v := range m {
		tags[k] = v
	}
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
