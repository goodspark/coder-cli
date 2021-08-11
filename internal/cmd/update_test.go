package cmd

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"cdr.dev/slog/sloggers/slogtest/assert"
	"github.com/blang/vfs/memfs"
	"golang.org/x/xerrors"
)

func Test_updater_run_noop(t *testing.T) {
	fakeVersion := "1.23.4"
	fakeCoderClient := &fakeUpdaterClient{}
	fakeCoderClient.APIVersionF = func(c context.Context) (string, error) {
		return fakeVersion, nil
	}
	ctx := context.Background()
	u := &updater{
		httpClient:     newFakeGetter("", 200, nil),
		coderClient:    fakeCoderClient,
		fs:             memfs.Create(),
		confirm:        fakeConfirmYes,
		tempdir:        "/tmp",
		executablePath: "/home/coder/bin/coder",
	}

	err := u.Run(ctx, true, fakeVersion)
	assert.Success(t, "", err)
}

func newFakeGetter(body string, code int, err error) getter {
	return &fakeGetter{
		resp: &http.Response{
			Body:       io.NopCloser(strings.NewReader(body)),
			StatusCode: code,
		},
		err: err,
	}
}

type fakeGetter struct {
	resp *http.Response
	err  error
}

func (f *fakeGetter) Get(url string) (*http.Response, error) {
	return f.resp, f.err
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

func fakeConfirmYes(_ string) (string, error) {
	return "y", nil
}

func fakeConfirmNo(_ string) (string, error) {
	return "n", nil
}

func fakeConfirmError(_ string) (string, error) {
	return "", xerrors.New("oops")
}
