package upstream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const commit = "c6dca50ea10fe5a23e750748582fddda263fb053"

func TestGitHubFetchesAndCaches(t *testing.T) {
	var hits atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		if r.URL.Path != "/networktocode/ntc-templates/"+commit+"/LICENSE" {
			http.NotFound(w, r)

			return
		}

		_, _ = w.Write([]byte("licence text"))
	}))
	defer server.Close()

	fetcher := NewGitHub(t.TempDir())
	fetcher.Base = server.URL

	for range 2 {
		body, err := fetcher.Fetch(context.Background(), "networktocode/ntc-templates", commit, "LICENSE")
		if err != nil || string(body) != "licence text" {
			t.Fatalf("Fetch = %q, %v", body, err)
		}
	}

	if hits.Load() != 1 {
		t.Errorf("fetched %d times, want 1 (second read from cache)", hits.Load())
	}

	_, err := fetcher.Fetch(context.Background(), "networktocode/ntc-templates", commit, "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("missing file: err = %v, want ErrNotFound", err)
	}
}

func TestGitHubRefusesShortCommitsAndEscapes(t *testing.T) {
	fetcher := NewGitHub("")

	for _, c := range []struct{ commit, file string }{
		{"c6dca50", "LICENSE"},
		{commit, "../../etc/passwd"},
	} {
		if _, err := fetcher.Fetch(context.Background(), "a/b", c.commit, c.file); err == nil {
			t.Errorf("Fetch(%q, %q) succeeded", c.commit, c.file)
		}
	}
}

func TestSelect(t *testing.T) {
	stream := []byte(`[{"data": "first"}]
[{"data": "zero"}, {"data": "leaf01 output\n", "n": 3}]`)

	got, err := Select(stream, "1.1.data")
	if err != nil || got != "leaf01 output\n" {
		t.Errorf("Select = %q, %v", got, err)
	}

	for _, path := range []string{"", "1", "x.0.data", "2.0.data", "1.5.data", "1.1.missing", "1.1.n", "0.0.data.deeper"} {
		if _, err := Select(stream, path); err == nil {
			t.Errorf("Select(%q) succeeded", path)
		}
	}
}
