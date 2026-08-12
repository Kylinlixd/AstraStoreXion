package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const defaultEnvFile = "/etc/astrastore-xion.env"

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "xionctl:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	commandIndex := findCommandIndex(args)
	if commandIndex < 0 {
		return errors.New(usage())
	}

	global := flag.NewFlagSet("xionctl", flag.ContinueOnError)
	global.SetOutput(stderr)
	envFile := global.String("env-file", defaultEnvFile, "Xion env 文件路径")
	apiURL := global.String("api", "", "Xion API 地址")
	token := global.String("token", "", "Xion 服务令牌")
	timeout := global.Duration("timeout", 0, "HTTP 请求超时")
	if err := global.Parse(args[:commandIndex]); err != nil {
		return err
	}
	command := args[commandIndex]
	commandArgs := args[commandIndex+1:]

	environment := map[string]string{}
	for _, key := range []string{"XION_API_URL", "XION_LISTEN_ADDR", "XION_SERVICE_TOKEN", "XION_TIMEOUT"} {
		environment[key] = os.Getenv(key)
	}
	cfg, err := loadConfig(*envFile, environment, configOverrides{apiURL: *apiURL, token: *token, timeout: *timeout})
	if err != nil {
		return err
	}
	client := newXionHTTPClient(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()

	switch command {
	case "health":
		status, err := client.health(ctx)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, status)
		return err
	case "list":
		flags, err := parseListFlags(commandArgs, stderr)
		if err != nil {
			return err
		}
		payload, err := client.list(ctx, flags.limit, flags.offset)
		if err != nil {
			return err
		}
		return printJSON(stdout, payload)
	case "info", "status":
		if len(commandArgs) != 1 || strings.TrimSpace(commandArgs[0]) == "" {
			return errors.New("用法: xionctl info <file-id>")
		}
		payload, err := client.info(ctx, commandArgs[0])
		if err != nil {
			return err
		}
		return printJSON(stdout, payload)
	case "upload":
		if len(commandArgs) != 1 || strings.TrimSpace(commandArgs[0]) == "" {
			return errors.New("用法: xionctl upload <file-path>")
		}
		payload, err := client.upload(ctx, commandArgs[0])
		if err != nil {
			return err
		}
		return printJSON(stdout, payload)
	case "download":
		if len(commandArgs) != 2 || strings.TrimSpace(commandArgs[0]) == "" || strings.TrimSpace(commandArgs[1]) == "" {
			return errors.New("用法: xionctl download <file-id> <output-path>")
		}
		if err := client.download(ctx, commandArgs[0], commandArgs[1]); err != nil {
			return err
		}
		_, err := fmt.Fprintf(stdout, "downloaded %s -> %s\n", commandArgs[0], commandArgs[1])
		return err
	case "delete":
		confirmed := false
		id := ""
		for _, arg := range commandArgs {
			if arg == "--yes" {
				confirmed = true
			} else if id == "" {
				id = arg
			} else {
				return errors.New("用法: xionctl delete <file-id> --yes")
			}
		}
		if id == "" {
			return errors.New("用法: xionctl delete <file-id> --yes")
		}
		if !confirmed {
			return errors.New("删除操作必须显式提供 --yes")
		}
		if err := client.delete(ctx, id); err != nil {
			return err
		}
		_, err := fmt.Fprintf(stdout, "deleted %s\n", id)
		return err
	default:
		return fmt.Errorf("未知命令 %q\n%s", command, usage())
	}
}

func findCommandIndex(args []string) int {
	globalOptionsWithValue := map[string]bool{
		"--env-file": true,
		"--api":      true,
		"--token":    true,
		"--timeout":  true,
	}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if strings.Contains(arg, "=") && strings.HasPrefix(arg, "--") {
			continue
		}
		if globalOptionsWithValue[arg] {
			index++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return index
	}
	return -1
}

type listFlags struct {
	limit  int
	offset int
}

func parseListFlags(args []string, stderr io.Writer) (listFlags, error) {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	flags.SetOutput(stderr)
	limit := flags.Int("limit", 100, "每页数量（1-1000）")
	offset := flags.Int("offset", 0, "跳过数量")
	if err := flags.Parse(args); err != nil {
		return listFlags{}, err
	}
	if flags.NArg() != 0 || *limit < 1 || *limit > 1000 || *offset < 0 {
		return listFlags{}, errors.New("用法: xionctl list [--limit 100] [--offset 0]")
	}
	return listFlags{limit: *limit, offset: *offset}, nil
}

func printJSON(writer io.Writer, payload []byte) error {
	if !isJSON(payload) {
		return errors.New("xion returned invalid JSON")
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return fmt.Errorf("parse xion JSON: %w", err)
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func usage() string {
	return "使用方法:\n  xionctl health\n  xionctl list [--limit 100] [--offset 0]\n  xionctl info <file-id>\n  xionctl upload <file-path>\n  xionctl download <file-id> <output-path>\n  xionctl delete <file-id> --yes\n\n全局选项:\n  --env-file <path>  默认 /etc/astrastore-xion.env\n  --api <url>        覆盖 Xion API 地址\n  --token <token>    覆盖服务令牌\n  --timeout <dur>    默认 30s"
}
