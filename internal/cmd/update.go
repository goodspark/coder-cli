package cmd

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"runtime"
	"time"

	"path/filepath"

	"cdr.dev/coder-cli/pkg/clog"
	"golang.org/x/xerrors"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

const downloadURLTemplate = "https://github.com/cdr/coder-cli/releases/download/%s/coder-cli-%s-%s.zip"

func updateCmd() *cobra.Command {
	var (
		force   bool
		version string
		query   bool
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update coder binary",
		Long:  "Update coder to the latest version, or to the correct version matching current login.",
		RunE:  doUpdate(force, query, version),
	}

	cmd.Flags().BoolVar(&force, "force", false, "do not prompt for confirmation")
	cmd.Flags().StringVar(&version, "version", "", "update to the specified version")

	return cmd
}

func canWrite(path string) bool {
	f, err := os.OpenFile(path, os.O_RDWR, 0666)
	defer func() { _ = f.Close() }()
	return err == nil
}

func doUpdate(force, query bool, version string) func(cmd *cobra.Command, _ []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		currentBinaryPath, err := filepath.Abs(os.Args[0])
		if err != nil {
			return clog.Fatal("preflight: failed to get path of current binary", clog.Causef("%w", err))
		}

		if !canWrite(currentBinaryPath) {
			return clog.Fatal(fmt.Sprintf("preflight: cannot open %q for writing", currentBinaryPath), clog.Causef("%w", err))
		}

		ctx := cmd.Context()

		client, err := newClient(ctx, false)
		if err != nil {
			return clog.Fatal("init http client", clog.Causef("%w", err))
		}

		clientBaseURL := client.BaseURL()

		if version == "" {
			apiVersion, err := client.APIVersion(ctx)
			if err != nil {
				return clog.Fatal("fetch api version", clog.Causef("%w", err))
			}
			version = apiVersion
		}

		if !force {
			confirm := promptui.Prompt{
				IsConfirm: true,
				Label:     fmt.Sprintf("Coder instance at %q reports version %s\nUpdate to this version?", (&clientBaseURL).String(), apiVersion),
			}
			if _, err := confirm.Run(); err != nil {
				return clog.Fatal("failed to confirm update", clog.BlankLine, clog.Tipf(`use "--force" to update without confirmation`))
			}
		}

		tempDir, err := ioutil.TempDir("", "coder-cli-update")
		if err != nil {
			return clog.Fatal("failed to create temp dir", clog.Causef("%w", err))
		}

		defer func() {
			_ = os.RemoveAll(tempDir)
		}()

		downloadURL := fmt.Sprintf(downloadURLTemplate, version, runtime.GOOS, runtime.GOARCH)
		downloadFilename := path.Base(downloadURL)
		downloadFilepath := path.Join(tempDir, downloadFilename)
		downloadFile, err := os.Create(downloadFilepath)
		if err != nil {
			return clog.Fatal(fmt.Sprintf("failed to create file: %s", downloadFilepath), clog.Causef("%w", err))
		}
		defer func() {
			_ = downloadFile.Close()
		}()

		bw := bufio.NewWriter(downloadFile)

		httpClient := &http.Client{
			Timeout: 10 * time.Second,
		}

		resp, err := httpClient.Get(downloadURL)
		if err != nil {
			return clog.Fatal(fmt.Sprintf("failed to fetch URL %s", downloadURL), clog.Causef("%w", err))
		}

		defer func() {
			resp.Body.Close()
		}()

		if _, err := io.Copy(bw, resp.Body); err != nil {
			return clog.Fatal(fmt.Sprintf("failed while downloading %s to %s", downloadURL, downloadFilepath), clog.Causef("%w", err))
		}

		// TODO: validate the checksum of the downloaded file. GitHub does not currently provide this information
		// and we do not generate them yet.

		zipReader, err := zip.OpenReader(downloadFilepath)
		if err != nil {
			return clog.Fatal(fmt.Sprintf("failed to open zip archive %s", downloadFilepath), clog.Causef("%w", err))
		}

		// We assume the binary is named coder
		updatedBinPath := path.Join(tempDir, "coder")
		var updatedBinZipFile *zip.File
		for _, f := range zipReader.File {
			if f.Name == "coder" {
				updatedBinZipFile = f
				break
			}
		}
		if updatedBinZipFile == nil {
			return xerrors.Errorf("could not find coder binary in downloaded zip archive")
		}

		rc, err := updatedBinZipFile.Open()
		if err != nil {
			return clog.Fatal("failed to extract updated coder binary from archive", clog.Causef("%w", err))
		}
		defer rc.Close()

		updatedBin, err := os.Create(updatedBinPath)
		if err != nil {
			return err
		}

		bw2 := bufio.NewWriter(updatedBin)
		lr := io.LimitReader(rc, 100*1024*1024)

		if _, err := io.Copy(bw2, lr); err != nil {
			return err
		}

		if err = os.Rename(updatedBinPath, os.Args[0]); err != nil {
			return err
		}

		clog.LogSuccess("Updated coder CLI to version " + version)
		return nil
	}
}
