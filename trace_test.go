package cloudtrace

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
	"time"

	"go.opencensus.io/trace"
)

func Test(t *testing.T) {

	var (
		proj = os.Getenv("PROJECT")
		err  error
	)
	ApplyConfig(proj, 1)
	http.DefaultTransport = BuildTraceRoundTripper(http.DefaultTransport)
	if err != nil {
		t.Error(err)
		return
	}

	fakeserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log(r.Header)
		w.Write([]byte("ok"))
	}))
	defer fakeserver.Close()

	target, _ := url.Parse(fakeserver.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = http.DefaultTransport

	// tags
	var m = map[string]string{
		"A": "aa",
		"B": "bb",
	}
	WithGlobalTags(m)
	var mm = Tags{}
	if err := mm.Set("C=cc"); err != nil {
		t.Error(err)
	}
	if err := mm.Set("D=dd"); err != nil {
		t.Error(err)
	}
	t.Log(mm)
	WithGlobalTags(mm)
	t.Log(mm.String())

	// only for get handler
	server := &http.Server{}
	ConfigureServer(server, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.FromContext(r.Context())
		t.Log(span)
		proxy.ServeHTTP(w, r)
	}), false, nil)

	request := httptest.NewRequest("GET", "http://aaa.bbb.ccc/xyz", nil)
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, request)

	Debug()
	time.Sleep(time.Second * 5)
}
