package agent

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// OpenCodeProvider implements Provider for the OpenCode agent backend.
type OpenCodeProvider struct {
	// binaryPath is the path to the opencode binary.
	// If empty, "opencode" is resolved via PATH.
	binaryPath string
}

// Verify interface compliance.
var _ Provider = (*OpenCodeProvider)(nil)

// NewOpenCodeProvider creates a new OpenCode provider.
// binaryPath can be empty to use the default "opencode" from PATH.
func NewOpenCodeProvider(binaryPath string) *OpenCodeProvider {
	if binaryPath == "" {
		binaryPath = "opencode"
	}
	return &OpenCodeProvider{binaryPath: binaryPath}
}

func (p *OpenCodeProvider) Name() string { return "opencode" }

// Ready checks that the opencode binary exists and is executable.
func (p *OpenCodeProvider) Ready(_ context.Context) error {
	_, err := exec.LookPath(p.binaryPath)
	if err != nil {
		return fmt.Errorf("binary %q not found: %w", p.binaryPath, err)
	}
	return nil
}

// Spawn starts an opencode serve process in the given working directory.
// It uses --port 0 to let the OS assign a free port, then parses the
// assigned port from stdout. Blocks until the health check passes.
func (p *OpenCodeProvider) Spawn(ctx context.Context, workDir string) (string, int, error) {
	// Use exec.Command (not CommandContext) — the process must outlive the
	// HTTP request that triggered the spawn. The registry manages the
	// process lifecycle via PID + SIGTERM in Stop().
	cmd := exec.Command(p.binaryPath, "serve", "--port", "0")
	cmd.Dir = workDir
	// Set process group so we can kill child processes on shutdown
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", 0, fmt.Errorf("opencode stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr // forward OpenCode stderr for debugging

	if err := cmd.Start(); err != nil {
		return "", 0, fmt.Errorf("opencode start: %w", err)
	}

	pid := cmd.Process.Pid

	// Parse the listening address from stdout.
	// OpenCode prints something like "Listening on http://127.0.0.1:XXXXX" on startup.
	baseURL, err := parseListenURL(ctx, stdout)
	if err != nil {
		// Kill the process if we can't parse the URL
		_ = cmd.Process.Kill()
		return "", 0, fmt.Errorf("opencode parse listen URL: %w", err)
	}

	// Wait for health check to pass
	if err := p.waitHealthy(ctx, baseURL); err != nil {
		_ = cmd.Process.Kill()
		return "", 0, fmt.Errorf("opencode health check: %w", err)
	}

	// Detach: don't wait for the process in this goroutine.
	// The registry manages the lifecycle via PID.
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Warn("opencode process exited", "pid", pid, "error", err)
		}
	}()

	slog.Info("OpenCode spawned", "pid", pid, "url", baseURL, "workdir", workDir)
	return baseURL, pid, nil
}

// Healthy checks if the OpenCode instance at baseURL is responsive.
func (p *OpenCodeProvider) Healthy(ctx context.Context, baseURL string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/global/health", nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Stop terminates the OpenCode process by PID via SIGTERM.
func (p *OpenCodeProvider) Stop(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		if err.Error() == "os: process already finished" {
			return nil
		}
		return fmt.Errorf("sigterm process %d: %w", pid, err)
	}
	slog.Info("sent SIGTERM to OpenCode process", "pid", pid)
	return nil
}

// NewAdapter creates an OpenCodeAdapter connected to the given baseURL.
func (p *OpenCodeProvider) NewAdapter(baseURL string) Adapter {
	return NewOpenCodeAdapter(baseURL)
}

// waitHealthy polls the health endpoint until it responds OK or the context is cancelled.
func (p *OpenCodeProvider) waitHealthy(ctx context.Context, baseURL string) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(15 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timed out waiting for %s to become healthy", baseURL)
		case <-ticker.C:
			if p.Healthy(ctx, baseURL) {
				return nil
			}
		}
	}
}

// parseListenURL reads stdout lines until it finds the listen URL.
// Expected format: a line containing "http://..." with the assigned port.
func parseListenURL(ctx context.Context, r interface{ Read([]byte) (int, error) }) (string, error) {
	scanner := bufio.NewScanner(r)
	deadline := time.After(10 * time.Second)

	type result struct {
		url string
		err error
	}
	ch := make(chan result, 1)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			// Look for a line containing an HTTP URL with a port
			if idx := strings.Index(line, "http://"); idx >= 0 {
				url := strings.TrimSpace(line[idx:])
				// Trim any trailing characters after the URL
				if spaceIdx := strings.IndexAny(url, " \t\n"); spaceIdx >= 0 {
					url = url[:spaceIdx]
				}
				ch <- result{url: strings.TrimRight(url, "/"), err: nil}
				return
			}
		}
		ch <- result{err: fmt.Errorf("stdout closed without printing listen URL")}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-deadline:
		return "", fmt.Errorf("timed out waiting for listen URL on stdout")
	case res := <-ch:
		return res.url, res.err
	}
}
