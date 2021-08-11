package cmd

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path"
	"runtime"
	"time"

	"cdr.dev/coder-cli/pkg/clog"
	"golang.org/x/xerrors"

	"github.com/blang/semver/v4"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

// TODO: check under following directories and warn if coder binary is under them:
//   * homebrew prefix
//   * coder assets root (env CODER_ASSETS_ROOT)

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
			return doUpdate(cmd.Context(), force, versionArg)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "do not prompt for confirmation")
	cmd.Flags().StringVar(&versionArg, "version", "", "update to the specified version")

	return cmd
}

func doUpdate(ctx context.Context, force bool, versionArg string) error {
	currentBinaryPath, err := os.Executable()
	if err != nil {
		return clog.Fatal("preflight: failed to get path of current binary", clog.Causef("%s", err))
	}

	currentBinaryStat, err := os.Stat(currentBinaryPath)
	if err != nil {
		return clog.Fatal("preflight: cannot stat current binary", clog.Causef("%s", err))
	}

	if currentBinaryStat.Mode().Perm()&0222 == 0 {
		return clog.Fatal("preflight: missing write permission on current binary")
	}

	brewPrefixCmd := exec.Command("brew", "--prefix")
	brewPrefixCmdOutput, err := brewPrefixCmd.CombinedOutput()
	if err != nil {
		clog.LogWarn("brew --prefix returned error", clog.Causef(err.Error()))
	} else {
		clog.LogInfo("brew --prefix returned output", string(brewPrefixCmdOutput))
	}

	client, err := newClient(ctx, false)
	if err != nil {
		return clog.Fatal("init http client", clog.Causef("%s", err))
	}

	var version semver.Version
	if versionArg == "" {
		apiVersion, err := client.APIVersion(ctx)
		if err != nil {
			return clog.Fatal("fetch api version", clog.Causef("%s", err))
		}
		version, err = semver.Make(apiVersion)
		if err != nil {
			return clog.Fatal("coder reported invalid version", clog.Causef(err.Error()))
		}
		clog.LogInfo(fmt.Sprintf("Coder instance at %q reports version %s", client.BaseURL().Host, version.FinalizeVersion()))
	} else {
		version, err = semver.Make(versionArg)
		if err != nil {
			return clog.Fatal("invalid version argument provided", clog.Causef(err.Error()))
		}
	}

	if !force {
		confirm := promptui.Prompt{
			IsConfirm: true,
			Label:     fmt.Sprintf("Update coder-cli to version %s?", version.FinalizeVersion()),
		}
		if _, err := confirm.Run(); err != nil {
			return clog.Fatal("failed to confirm update", clog.BlankLine, clog.Tipf(`use "--force" to update without confirmation`))
		}
	}

	tempDir, err := ioutil.TempDir("", "coder-cli-update")
	if err != nil {
		return clog.Fatal("failed to create temp dir", clog.Causef("%s", err))
	}

	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	downloadURL := makeDownloadURL(version.FinalizeVersion(), runtime.GOOS, runtime.GOARCH)
	downloadFilename := path.Base(downloadURL)
	downloadFilepath := path.Join(tempDir, downloadFilename)
	downloadFile, err := os.Create(downloadFilepath)
	if err != nil {
		return clog.Fatal(fmt.Sprintf("failed to create file: %s", downloadFilepath), clog.Causef("%s", err))
	}
	defer func() {
		_ = downloadFile.Close()
	}()

	bw := bufio.NewWriter(downloadFile)

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	clog.LogInfo("fetching coder-cli from GitHub releases", downloadURL)
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return clog.Fatal(fmt.Sprintf("failed to fetch URL %s", downloadURL), clog.Causef("%s", err))
	}

	if resp.StatusCode != http.StatusOK {
		return clog.Fatal("failed to fetch release", clog.Causef("URL %s returned status code %d", downloadURL, resp.StatusCode))
	}

	defer func() {
		resp.Body.Close()
	}()

	if _, err := io.Copy(bw, resp.Body); err != nil {
		return clog.Fatal(fmt.Sprintf("failed while downloading %s to %s", downloadURL, downloadFilepath), clog.Causef("%s", err))
	}

	// TODO: validate the checksum of the downloaded file. GitHub does not currently provide this information
	// and we do not generate them yet.

	zipReader, err := zip.OpenReader(downloadFilepath)
	if err != nil {
		return clog.Fatal(fmt.Sprintf("failed to open zip archive %s", downloadFilepath), clog.Causef("%s", err))
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
		return clog.Fatal("failed to extract updated coder binary from archive", clog.Causef("%s", err))
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

	if err = os.Rename(updatedBinPath, currentBinaryPath); err != nil {
		return err
	}

	clog.LogSuccess("Updated coder CLI to version " + version.FinalizeVersion())
	return nil
}
