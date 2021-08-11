package cmd

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"cdr.dev/coder-cli/internal/coderutil"
	"cdr.dev/slog/sloggers/slogtest/assert"
)

func Test_updater_run_noop(t *testing.T) {
	fakeVersion := "1.23.4"
	fakeCoderClient := &fakeUpdaterClient{}
	fakeCoderClient.APIVersionF = func(c context.Context) (string, error) {
		return fakeVersion, nil
	}
	fakeHTTPClient := newFakeGetter("", 200, nil)
	fakeOS := newFakeOS()
	ctx := context.Background()
	u := &updater{
		httpClient:  fakeHTTPClient,
		coderClient: fakeCoderClient,
		os:          fakeOS,
	}

	err := u.Run(ctx, true, fakeVersion)
	assert.Success(t, "", err)
}

type fakeOS struct {
	fs fstest.MapFS
}



func newFakeOS() *coderutil.OS {
	return &coderutil.OS{
		CreateF: func(_ string) (io.ReadWriteCloser, error) {
			return &MemReadAtWriteCloser{}, nil
		},
		ExecCommandF: func(_ string, _ ...string) ([]byte, error) {
			return []byte{}, nil
		},
		ExecutableF: func() (string, error) {
			return "", nil
		},
		ModeF: func(s string) (fs.FileMode, error) {
			return fs.FileMode(0644), nil
		},
		RemoveAllF: func(s string) error {
			return nil
		},
		TempDirF: func(s1, s2 string) (string, error) {
			return "", nil
		},
	}
}

func newFakeGetter(body string, code int, err error) getter {
	resp := &http.Response{
		Body:       io.NopCloser(strings.NewReader(body)),
		StatusCode: code,
	}
	return &fakeGetter{
		GetF: func(_ string) (*http.Response, error) {
			return resp, err
		},
	}
}

type fakeGetter struct {
	GetF func(url string) (*http.Response, error)
}

func (f *fakeGetter) Get(url string) (*http.Response, error) {
	return f.GetF(url)
}

type fakeUpdaterClient struct {
	APIVersionF func(context.Context) (string, error)
	BaseURLF    func() url.URL
}

func (f *fakeUpdaterClient) APIVersion(ctx context.Context) (string, error) {
	return f.APIVersionF(ctx)
}

func (f *fakeUpdaterClient) BaseURL() url.URL {
	return f.BaseURLF()
}

type MemReadAtWriteCloser struct {
	B *bytes.Buffer
}

func (m *MemReadAtWriteCloser) Read(p []byte) (int, error) {
	return m.B.Read(p)
}

func (m *MemReadAtWriteCloser) ReadAt(p []byte, off int64) (n int, err error) {
	return bytes.NewReader(m.B.Bytes()).ReadAt(p, off)
}

func (m *MemReadAtWriteCloser) Write(p []byte) (int, error) {
	return m.B.Write(p)
}

func (m *MemReadAtWriteCloser) Close() error {
	return nil
}
