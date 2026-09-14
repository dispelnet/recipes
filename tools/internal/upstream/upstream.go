// Package upstream fetches files from upstream projects at a fixed commit, and
// picks one value out of a file that is a stream of JSON documents.
package upstream

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Fetcher returns a file from a GitHub repository at a commit.
type Fetcher interface {
	Fetch(ctx context.Context, source, commit, file string) ([]byte, error)
}

// ErrNotFound means the upstream answered, and the file is not there.
var ErrNotFound = errors.New("not found upstream")

// maxFile caps what is read for one upstream file.
const maxFile = 64 << 20

// FullCommit is the only commit form accepted: a short SHA or a tag can come to
// name something else, and provenance that can move proves nothing.
var FullCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

// GitHub fetches from raw.githubusercontent.com, keeping what it fetched in a
// directory. Content at a full commit cannot change, so the cache never
// expires.
type GitHub struct {
	Cache  string
	Client *http.Client
	Base   string // overridden in tests
}

// NewGitHub returns a Fetcher caching under dir. An empty dir disables caching.
func NewGitHub(dir string) *GitHub {
	return &GitHub{
		Cache:  dir,
		Client: &http.Client{Timeout: 60 * time.Second},
		Base:   "https://raw.githubusercontent.com",
	}
}

// Fetch returns file from source at commit, from the cache when it holds it. A
// file that is not there upstream is ErrNotFound and is not retried.
func (g *GitHub) Fetch(ctx context.Context, source, commit, file string) ([]byte, error) {
	if !FullCommit.MatchString(commit) {
		return nil, fmt.Errorf("commit %q is not a full 40-character SHA", commit)
	}

	if strings.Contains(source, "..") || strings.Contains(file, "..") {
		return nil, fmt.Errorf("refusing a path containing ..: %s %s", source, file)
	}

	cached := g.cachePath(source, commit, file)
	if cached != "" {
		if body, err := os.ReadFile(cached); err == nil { //nolint:gosec // G304: a hash-named file in the cache directory
			return body, nil
		}
	}

	url := strings.TrimRight(g.Base, "/") + "/" + source + "/" + commit + "/" + file

	var lastErr error

	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}

		body, err := g.get(ctx, url)
		if err == nil {
			g.store(cached, body)

			return body, nil
		}

		if errors.Is(err, ErrNotFound) {
			return nil, err
		}

		lastErr = err
	}

	return nil, lastErr
}

func (g *GitHub) get(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	response, err := g.Client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = response.Body.Close() }()

	switch {
	case response.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s: %w", url, ErrNotFound)
	case response.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("fetching %s: %s", url, response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxFile+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}

	if len(body) > maxFile {
		return nil, fmt.Errorf("%s is larger than %d bytes", url, maxFile)
	}

	return body, nil
}

func (g *GitHub) cachePath(source, commit, file string) string {
	if g.Cache == "" {
		return ""
	}

	sum := sha256.Sum256([]byte(source + "\x00" + commit + "\x00" + file))

	return filepath.Join(g.Cache, hex.EncodeToString(sum[:]))
}

// store writes to the cache on a best-effort basis: a cache that cannot be
// written only costs a fetch next time.
func (g *GitHub) store(path string, body []byte) {
	if path == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}

	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, body, 0o600); err != nil {
		return
	}

	_ = os.Rename(temporary, path)
}

// Select picks one string out of a stream of JSON documents.
//
// The path is dot-separated: the first segment is the document's index in the
// stream, then array indexes and object keys. "1.1.data" is the data field of
// the second element of the second document. The value must be a string,
// because a capture is text a device printed.
func Select(stream []byte, path string) (string, error) {
	segments := strings.Split(path, ".")
	if path == "" || len(segments) < 2 {
		return "", fmt.Errorf("select %q needs a document index and at least one more segment", path)
	}

	index, err := strconv.Atoi(segments[0])
	if err != nil || index < 0 {
		return "", fmt.Errorf("select %q must start with a document index", path)
	}

	decoder := json.NewDecoder(bytes.NewReader(stream))

	var current any

	for i := 0; ; i++ {
		var document any
		if err := decoder.Decode(&document); err != nil {
			if errors.Is(err, io.EOF) {
				return "", fmt.Errorf("select %q: the stream has only %d documents", path, i)
			}

			return "", fmt.Errorf("select %q: document %d is not JSON: %w", path, i, err)
		}

		if i == index {
			current = document

			break
		}
	}

	for n, segment := range segments[1:] {
		walked := strings.Join(segments[:n+2], ".")

		switch value := current.(type) {
		case []any:
			i, err := strconv.Atoi(segment)
			if err != nil || i < 0 || i >= len(value) {
				return "", fmt.Errorf("select %q: %s is not an index into an array of %d", path, walked, len(value))
			}

			current = value[i]
		case map[string]any:
			next, ok := value[segment]
			if !ok {
				return "", fmt.Errorf("select %q: %s has no such key", path, walked)
			}

			current = next
		default:
			return "", fmt.Errorf("select %q: cannot descend into %s", path, walked)
		}
	}

	text, ok := current.(string)
	if !ok {
		return "", fmt.Errorf("select %q does not name a string", path)
	}

	return text, nil
}
