package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

const xionSystemdUnit = "astrastore-xion.service"

type logsFlags struct {
	lines  int
	since  string
	follow bool
}

type journalRunner func(context.Context, io.Writer, io.Writer, []string) error
type serviceRunner func(context.Context, ...string) error

func parseLogsFlags(args []string, stderr io.Writer) (logsFlags, error) {
	flags := newFlagSet("logs", stderr)
	lines := flags.Int("lines", 100, "显示最近多少行（1-10000）")
	since := flags.String("since", "", "只显示指定时间之后的日志，例如 1h、today")
	follow := flags.Bool("follow", false, "持续跟踪新日志")
	if err := flags.Parse(args); err != nil {
		return logsFlags{}, err
	}
	if flags.NArg() != 0 || *lines < 1 || *lines > 10000 {
		return logsFlags{}, errors.New("用法: xionctl logs [--lines 100] [--since 1h] [--follow]")
	}
	return logsFlags{lines: *lines, since: *since, follow: *follow}, nil
}

func runLogs(ctx context.Context, flags logsFlags, stdout, stderr io.Writer, runner journalRunner) error {
	args := []string{"-u", xionSystemdUnit, "--no-pager", "--output=cat", "-n", strconv.Itoa(flags.lines)}
	if flags.since != "" {
		args = append(args, "--since", flags.since)
	}
	if flags.follow {
		args = append(args, "-f")
	}
	return runner(ctx, stdout, stderr, args)
}

func journalctlRunner(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	command := exec.CommandContext(ctx, "journalctl", args...)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return fmt.Errorf("journalctl: %w", err)
	}
	return nil
}

func systemctlRunner(ctx context.Context, args ...string) error {
	command := exec.CommandContext(ctx, "systemctl", args...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
