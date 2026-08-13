package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultMaxUploadBytes int64 = 50 << 20

func runConfig(envPath string, args []string, stdout, stderr io.Writer, serviceRunner serviceRunner) error {
	flags := newFlagSet("config", stderr)
	maxUploadSize := flags.String("max-upload-size", "", "最大上传大小，例如 50M、1G")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("用法: xionctl config [--max-upload-size 100M]")
	}

	values, err := readEnvFile(envPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("读取配置失败: %w", err)
	}
	current := defaultMaxUploadBytes
	if raw := values["XION_MAX_UPLOAD_BYTES"]; raw != "" {
		current, err = parseUploadSize(raw)
		if err != nil {
			return fmt.Errorf("配置中的 XION_MAX_UPLOAD_BYTES 无效: %w", err)
		}
	}
	if strings.TrimSpace(*maxUploadSize) == "" {
		_, err = fmt.Fprintf(stdout, "Xion 配置\n\n最大上传：%s\n配置文件：%s\n", humanBytes(uint64(current)), envPath)
		return err
	}

	updated, err := parseUploadSize(*maxUploadSize)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		return fmt.Errorf("读取配置文件失败: %w", err)
	}
	backupPath := fmt.Sprintf("%s.backup-%s", envPath, time.Now().UTC().Format("20060102-150405"))
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return fmt.Errorf("创建配置备份失败: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(envPath); statErr == nil {
		mode = info.Mode().Perm()
		if mode == 0 {
			mode = 0o600
		}
	}
	newData := setEnvValue(data, "XION_MAX_UPLOAD_BYTES", strconv.FormatInt(updated, 10))
	temporary, err := os.CreateTemp(filepath.Dir(envPath), ".astrastore-xion.env-*")
	if err != nil {
		return fmt.Errorf("创建临时配置失败: %w", err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置配置权限失败: %w", err)
	}
	if _, err := temporary.Write(newData); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入临时配置失败: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("同步临时配置失败: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭临时配置失败: %w", err)
	}
	if err := os.Rename(temporaryPath, envPath); err != nil {
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	removeTemporary = false

	if err := serviceRunner(context.Background(), "restart", xionSystemdUnit); err != nil {
		_ = os.WriteFile(envPath, data, mode)
		_ = serviceRunner(context.Background(), "restart", xionSystemdUnit)
		return fmt.Errorf("重启 Xion 失败，已恢复旧配置: %w", err)
	}
	_, err = fmt.Fprintf(stdout, "已更新最大上传：%s\nXion 服务已重启\n备份配置：%s\n", humanBytes(uint64(updated)), backupPath)
	return err
}

func parseUploadSize(raw string) (int64, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if value == "" {
		return 0, errors.New("最大上传大小不能为空")
	}
	multiplier := int64(1)
	for _, suffix := range []struct {
		name       string
		multiplier int64
	}{{"KIB", 1 << 10}, {"KB", 1 << 10}, {"K", 1 << 10}, {"MIB", 1 << 20}, {"MB", 1 << 20}, {"M", 1 << 20}, {"GIB", 1 << 30}, {"GB", 1 << 30}, {"G", 1 << 30}, {"TIB", 1 << 40}, {"TB", 1 << 40}, {"T", 1 << 40}} {
		if strings.HasSuffix(value, suffix.name) {
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix.name))
			multiplier = suffix.multiplier
			break
		}
	}
	base, err := strconv.ParseInt(value, 10, 64)
	if err != nil || base <= 0 || base > (1<<62)/multiplier {
		return 0, fmt.Errorf("最大上传大小无效: %q（示例：50M、1G）", raw)
	}
	return base * multiplier, nil
}

func setEnvValue(data []byte, key, value string) []byte {
	lines := strings.Split(string(data), "\n")
	found := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, key+"=") {
			lines[index] = key + "=" + value
			found = true
		}
	}
	if !found {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, key+"="+value)
	}
	return []byte(strings.Join(lines, "\n"))
}
