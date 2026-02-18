package cmd

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"

	"github.com/fertile-org/banyan/pkg/types"
)

// NerdctlLogProvider streams container logs using nerdctl.
type NerdctlLogProvider struct{}

func (p *NerdctlLogProvider) StreamLogs(ctx context.Context, containerName string, opts types.LogOptions) (io.ReadCloser, error) {
	args := []string{"logs"}

	if opts.Follow {
		args = append(args, "-f")
	}
	if opts.Tail > 0 {
		args = append(args, "--tail", strconv.Itoa(opts.Tail))
	}

	args = append(args, containerName)

	cmd := exec.CommandContext(ctx, "nerdctl", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start nerdctl logs: %w", err)
	}

	return &cmdReadCloser{cmd: cmd, reader: stdout}, nil
}

// cmdReadCloser wraps an exec.Cmd and its stdout, cleaning up the process on Close.
type cmdReadCloser struct {
	cmd    *exec.Cmd
	reader io.ReadCloser
}

func (c *cmdReadCloser) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func (c *cmdReadCloser) Close() error {
	c.reader.Close()
	if c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	c.cmd.Wait()
	return nil
}
