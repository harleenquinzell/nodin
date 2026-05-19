package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

const (
	modulePath  = "github.com/harleenquinzell/nodin"
	installPath = modulePath + "/cmd/nodin@latest"
)

func newUpdateCmd(currentVersion string) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update nodin to the latest version",
		Long:  "Checks the latest release and runs go install to update.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), currentVersion)
		},
	}
}

func runUpdate(ctx context.Context, currentVersion string) error {
	goExe, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go not found on PATH; install Go from https://go.dev/dl/ first")
	}

	fmt.Printf("current: %s\n", currentVersion)

	latest, err := fetchLatestVersion(ctx)
	if err != nil {
		fmt.Printf("latest:  (could not check: %v)\n", err)
	} else {
		fmt.Printf("latest:  %s\n", latest)
		if currentVersion != "dev" && currentVersion == latest {
			fmt.Println("already up to date")
			return nil
		}
	}

	fmt.Printf("\nrunning: go install %s\n\n", installPath)

	cmd := exec.CommandContext(ctx, goExe, "install", installPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	fmt.Println("\ndone")
	return nil
}

func fetchLatestVersion(ctx context.Context) (string, error) {
	url := "https://proxy.golang.org/" + modulePath + "/@latest"
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var info struct {
		Version string `json:"Version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if info.Version == "" {
		return "", fmt.Errorf("empty version in response")
	}
	return info.Version, nil
}
