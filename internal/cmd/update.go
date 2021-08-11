package cmd

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"time"

	"cdr.dev/coder-cli/coder-sdk"
	"cdr.dev/coder-cli/internal/coderutil"
	"cdr.dev/coder-cli/pkg/clog"
	"golang.org/x/xerrors"

	"github.com/blang/semver/v4"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

// TODO: check under following directories and warn if coder binary is under them:
//   * homebrew prefix
//   * coder assets root (env CODER_ASSETS_ROOT)

func updateCmd() *cobra.Command {
	var (
		force      bool
		versionArg string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update coder binary",
		Long:  "Update coder to the latest version, or to the correct version matching current login.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			client, err := newClient(ctx, false)
			if err != nil {
				return clog.Fatal("could not init coder client", clog.Causef(err.Error()))
			}
			updater := &updater{
				httpClient: &http.Client{
					Timeout: 10 * time.Second,
				},
				coderClient: client,
				os:          coderutil.NewDefaultOS(),
				confirm:     defaultConfirm,
			}
			return updater.Run(ctx, force, versionArg)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "do not prompt for confirmation")
	cmd.Flags().StringVar(&versionArg, "version", "", "update to the specified version")

	return cmd
}

// updaterClient specifies the methods of coder.Client used by updater
type updaterClient interface {
	APIVersion(context.Context) (string, error)
	BaseURL() url.URL
}

// ensure that we have the methods we need
var _ updaterClient = &coder.DefaultClient{}

// updater updates coder-cli
type updater struct {
	httpClient  getter
	coderClient updaterClient
	os          coderutil.OSer
	confirm     func(label string) (string, error)
}

type getter interface {
	Get(url string) (*http.Response, error)
}

func (u *updater) Run(ctx context.Context, force bool, versionArg string) error {
	currentBinaryPath, err := u.os.Executable()
	if err != nil {
		return clog.Fatal("preflight: failed to get path of current binary", clog.Causef("%s", err))
	}

	currentBinaryStat, err := u.os.Stat(currentBinaryPath)
	if err != nil {
		return clog.Fatal("preflight: cannot stat current binary", clog.Causef("%s", err))
	}

	if currentBinaryStat.Mode().Perm()&0222 == 0 {
		return clog.Fatal("preflight: missing write permission on current binary")
	}

	var version semver.Version
	if versionArg == "" {
		apiVersion, err := u.coderClient.APIVersion(ctx)
		if err != nil {
			return clog.Fatal("fetch api version", clog.Causef(err.Error()))
		}
		version, err = semver.Make(apiVersion)
		if err != nil {
			return clog.Fatal("coder reported invalid version", clog.Causef(err.Error()))
		}
		clog.LogInfo(fmt.Sprintf("Coder instance at %q reports version %s", u.coderClient.BaseURL().Host, version.FinalizeVersion()))
	} else {
		version, err = semver.Make(versionArg)
		if err != nil {
			return clog.Fatal("invalid version argument provided", clog.Causef(err.Error()))
		}
	}

	if !force {
		label := fmt.Sprintf("Update coder-cli to version %s?", version.FinalizeVersion())
		if _, err := u.confirm(label); err != nil {
			return clog.Fatal("failed to confirm update", clog.Tipf(`use "--force" to update without confirmation`))
		}
	}

	downloadURL := makeDownloadURL(version.FinalizeVersion(), runtime.GOOS, runtime.GOARCH)

	var downloadBuf bytes.Buffer
	memWriter := bufio.NewWriter(&downloadBuf)

	clog.LogInfo("fetching coder-cli from GitHub releases", downloadURL)
	resp, err := u.httpClient.Get(downloadURL)
	if err != nil {
		return clog.Fatal(fmt.Sprintf("failed to fetch URL %s", downloadURL), clog.Causef(err.Error()))
	}

	if resp.StatusCode != http.StatusOK {
		return clog.Fatal("failed to fetch release", clog.Causef("URL %s returned status code %d", downloadURL, resp.StatusCode))
	}

	defer func() {
		resp.Body.Close()
	}()

	if _, err := io.Copy(memWriter, resp.Body); err != nil {
		return clog.Fatal(fmt.Sprintf("failed to download %s", downloadURL), clog.Causef(err.Error()))
	}

	if err := memWriter.Flush(); err != nil {
		return clog.Fatal(fmt.Sprintf("failed to save %s", downloadURL), clog.Causef(err.Error()))
	}

	// TODO: validate the checksum of the downloaded file. GitHub does not currently provide this information
	// and we do not generate them yet.
	// zipReader, err := zip.NewReader(bytes.NewReader(downloadBuf.Bytes()), n)
	// if err != nil {
	// 	return clog.Fatal("failed to open zip archive", clog.Causef(err.Error()))
	// }

	// var zf *zip.File
	// for _, f := range zipReader.File {
	// 	if f.Name == "coder" {
	// 		zf = f
	// 		break
	// 	}
	// }
	// if zf == nil {
	// 	return xerrors.Errorf("could not find coder binary in downloaded zip archive")
	// }

	// zipReadCloser, err := zf.Open()
	// if err != nil {
	// 	return clog.Fatal("failed to extract updated coder binary from archive", clog.Causef(err.Error()))
	// }
	// defer zipReadCloser.Close()
	updatedBinary, err := extractFromArchive("coder", downloadBuf.Bytes())
	if err != nil {
		return clog.Fatal("failed to extract coder binary from archive", clog.Causef(err.Error()))
	}

	// We assume the binary is named coder and write it to $TEMPDIR/coder
	updatedCoderBinaryPath := path.Join(u.os.TempDir(), "coder")
	updatedBin, err := u.os.Create(updatedCoderBinaryPath)
	if err != nil {
		return clog.Fatal("failed to create file for updated coder binary", clog.Causef(err.Error()))
	}

	fsWriter := bufio.NewWriter(updatedBin)
	if _, err := io.Copy(fsWriter, bytes.NewReader(updatedBinary)); err != nil {
		return clog.Fatal("failed to write updated coder binary to disk", clog.Causef(err.Error()))
	}

	if err = u.os.Rename(updatedCoderBinaryPath, currentBinaryPath); err != nil {
		return clog.Fatal("failed to update coder binary in-place", clog.Causef(err.Error()))
	}

	clog.LogSuccess("Updated coder CLI to version " + version.FinalizeVersion())
	return nil
}

func defaultConfirm(label string) (string, error) {
	p := promptui.Prompt{IsConfirm: true, Label: label}
	return p.Run()
}

func makeDownloadURL(version, ostype, archtype string) string {
	const template = "https://github.com/cdr/coder-cli/releases/download/v%s/coder-cli-%s-%s.%s"
	var ext string
	switch ostype {
	case "linux":
		ext = "tar.gz"
	default:
		ext = ".zip"
	}
	return fmt.Sprintf(template, version, ostype, archtype, ext)
}

func extractFromArchive(path string, archive []byte) ([]byte, error) {
	contentType := http.DetectContentType(archive)
	switch contentType {
	case "application/zip":
		return extractFromZipArchive(path, archive)
	case "application/x-gzip":
		return extractFromTGZArchive(path, archive)
	default:
		return nil, xerrors.Errorf("unknown archive type: %s", contentType)
	}
}

func extractFromZipArchive(path string, archive []byte) ([]byte, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, xerrors.Errorf("failed to open zip archive")
	}

	var zf *zip.File
	for _, f := range zipReader.File {
		if f.Name == path {
			zf = f
			break
		}
	}
	if zf == nil {
		return nil, xerrors.Errorf("could not find path %q in zip archive", path)
	}

	zipReadCloser, err := zf.Open()
	if err != nil {
		return nil, xerrors.Errorf("failed to extract path %q from archive", path)
	}
	defer zipReadCloser.Close()

	var b bytes.Buffer
	bw := bufio.NewWriter(&b)
	if _, err := io.Copy(bw, zipReadCloser); err != nil {
		return nil, xerrors.Errorf("failed to copy path %q to from archive", path)
	}
	return b.Bytes(), nil
}

func extractFromTGZArchive(path string, archive []byte) ([]byte, error) {
	return nil, nil
}
