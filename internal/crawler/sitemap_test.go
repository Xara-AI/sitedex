package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func TestFetchSitemapURLs_Flat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>` + "http://" + r.Host + `/a</loc></url>
  <url><loc>` + "http://" + r.Host + `/b</loc></url>
</urlset>`))
	}))
	defer srv.Close()

	urls := FetchSitemapURLs(context.Background(), srv.Client(), "sitedex-test", srv.URL+"/sitemap.xml")
	sort.Strings(urls)
	want := []string{srv.URL + "/a", srv.URL + "/b"}
	sort.Strings(want)
	if len(urls) != 2 || urls[0] != want[0] || urls[1] != want[1] {
		t.Errorf("urls = %v, want %v", urls, want)
	}
}

func TestFetchSitemapURLs_Index(t *testing.T) {
	mux := http.NewServeMux()
	var baseURL string
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>` + baseURL + `/sitemap-1.xml</loc></sitemap>
  <sitemap><loc>` + baseURL + `/sitemap-2.xml</loc></sitemap>
</sitemapindex>`))
	})
	mux.HandleFunc("/sitemap-1.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<urlset><url><loc>` + baseURL + `/p1</loc></url></urlset>`))
	})
	mux.HandleFunc("/sitemap-2.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<urlset><url><loc>` + baseURL + `/p2</loc></url></urlset>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	urls := FetchSitemapURLs(context.Background(), srv.Client(), "sitedex-test", srv.URL+"/sitemap.xml")
	sort.Strings(urls)
	want := []string{srv.URL + "/p1", srv.URL + "/p2"}
	if len(urls) != 2 || urls[0] != want[0] || urls[1] != want[1] {
		t.Errorf("urls = %v, want %v", urls, want)
	}
}

func TestFetchSitemapURLs_NotFoundIsTolerant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	urls := FetchSitemapURLs(context.Background(), srv.Client(), "sitedex-test", srv.URL+"/sitemap.xml")
	if len(urls) != 0 {
		t.Errorf("urls = %v, want empty on 404", urls)
	}
}

func TestFetchSitemapURLs_MalformedXMLIsTolerant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<not><valid"))
	}))
	defer srv.Close()

	urls := FetchSitemapURLs(context.Background(), srv.Client(), "sitedex-test", srv.URL+"/sitemap.xml")
	if len(urls) != 0 {
		t.Errorf("urls = %v, want empty on malformed XML", urls)
	}
}

func TestFetchSitemapURLs_SelfReferencingIndexDoesNotLoop(t *testing.T) {
	mux := http.NewServeMux()
	var baseURL string
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<sitemapindex><sitemap><loc>` + baseURL + `/sitemap.xml</loc></sitemap></sitemapindex>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	baseURL = srv.URL

	done := make(chan []string, 1)
	go func() {
		done <- FetchSitemapURLs(context.Background(), srv.Client(), "sitedex-test", srv.URL+"/sitemap.xml")
	}()
	select {
	case urls := <-done:
		if len(urls) != 0 {
			t.Errorf("urls = %v, want empty", urls)
		}
	case <-testTimeout(t):
		t.Fatal("FetchSitemapURLs did not return, likely stuck in a loop")
	}
}
