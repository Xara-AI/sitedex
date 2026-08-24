package crawler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchPage_Basic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "sitedex-test-agent" {
			t.Errorf("User-Agent = %q, want sitedex-test-agent", r.Header.Get("User-Agent"))
		}
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>hi</body></html>"))
	}))
	defer srv.Close()

	res, err := FetchPage(context.Background(), NewHTTPClient(), "sitedex-test-agent", srv.URL, "", "")
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", res.StatusCode)
	}
	if res.ETag != `"abc123"` {
		t.Errorf("ETag = %q, want \"abc123\"", res.ETag)
	}
	if string(res.Body) != "<html><body>hi</body></html>" {
		t.Errorf("Body = %q", res.Body)
	}
}

func TestFetchPage_ConditionalGetReturnsNotModified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()

	res, err := FetchPage(context.Background(), NewHTTPClient(), "sitedex-test", srv.URL, `"v1"`, "")
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if !res.NotModified {
		t.Error("expected NotModified=true")
	}
	if len(res.Body) != 0 {
		t.Errorf("expected empty body on 304, got %q", res.Body)
	}
}

func TestFetchPage_FollowsRedirect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/new", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/new", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("landed"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := FetchPage(context.Background(), NewHTTPClient(), "sitedex-test", srv.URL+"/old", "", "")
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	if string(res.Body) != "landed" {
		t.Errorf("Body = %q, want landed", res.Body)
	}
	if res.FinalURL != srv.URL+"/new" {
		t.Errorf("FinalURL = %q, want %s/new", res.FinalURL, srv.URL)
	}
}

func TestFetchPage_RedirectLoop(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/b", http.StatusFound)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/a", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := FetchPage(context.Background(), NewHTTPClient(), "sitedex-test", srv.URL+"/a", "", "")
	if !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("err = %v, want ErrTooManyRedirects", err)
	}
}
