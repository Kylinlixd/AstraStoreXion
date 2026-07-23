package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/astrastore/astrastore-xion/pkg/auth"
	"github.com/astrastore/astrastore-xion/pkg/files"
	"github.com/gorilla/mux"
)

var (
	authenticator auth.Authenticator
	authorizer    auth.Authorizer
	fileService   *files.Service
)

// 上传文件处理函数
func uploadFile(w http.ResponseWriter, r *http.Request) {
	// 验证权限
	user, err := getUserFromRequest(r)
	if err != nil {
		handleError(w, "认证失败", err, http.StatusUnauthorized)
		return
	}

	// 检查权限
	hasPermission, err := authorizer.CheckPermission(r.Context(), user, auth.WritePermission, "files")
	if err != nil || !hasPermission {
		handleError(w, "权限不足", err, http.StatusForbidden)
		return
	}

	// 解析多部分表单
	err = r.ParseMultipartForm(32 << 20) // 32MB 限制
	if err != nil {
		handleError(w, "解析表单失败", err, http.StatusBadRequest)
		return
	}

	// 获取文件
	file, header, err := r.FormFile("file")
	if err != nil {
		handleError(w, "获取文件失败", err, http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 解析元数据
	fileMetadata := make(map[string]string)
	if metadataStr := r.FormValue("metadata"); metadataStr != "" {
		if err := json.Unmarshal([]byte(metadataStr), &fileMetadata); err != nil {
			handleError(w, "解析元数据失败", err, http.StatusBadRequest)
			return
		}
	}

	service := ensureFileService()
	created, err := service.Upload(r.Context(), files.UploadInput{
		Name:        header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		OwnerID:     user.ID,
		Reader:      file,
	})
	if err != nil {
		handleError(w, "保存文件失败", err, http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	resp := map[string]interface{}{
		"file_id":      created.ID,
		"id":           created.ID,
		"name":         created.Name,
		"size":         created.Size,
		"checksum":     created.Checksum,
		"content_type": created.ContentType,
		"created_at":   created.CreatedAt.Format(time.RFC3339),
		"download_url": fmt.Sprintf("/api/v1/files/%s", created.ID),
		"success":      true,
		"message":      "文件上传成功",
	}

	respondJSON(w, resp, http.StatusOK)
}

// 下载文件处理函数
func downloadFile(w http.ResponseWriter, r *http.Request) {
	// 验证权限
	user, err := getUserFromRequest(r)
	if err != nil {
		handleError(w, "认证失败", err, http.StatusUnauthorized)
		return
	}

	// 获取文件ID
	vars := mux.Vars(r)
	fileID := vars["id"]
	if fileID == "" {
		handleError(w, "缺少文件ID", nil, http.StatusBadRequest)
		return
	}

	// 检查权限
	hasPermission, err := authorizer.CheckPermission(r.Context(), user, auth.ReadPermission, "files")
	if err != nil || !hasPermission {
		handleError(w, "权限不足", err, http.StatusForbidden)
		return
	}

	download, err := ensureFileService().Download(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, files.ErrNotFound) {
			handleError(w, "文件不存在", err, http.StatusNotFound)
			return
		}
		handleError(w, "读取文件失败", err, http.StatusInternalServerError)
		return
	}
	defer download.Content.Close()

	// 设置响应头
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", download.File.Name))
	w.Header().Set("Content-Type", download.File.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", download.File.Size))

	// 写入文件内容
	if _, err := io.Copy(w, download.Content); err != nil {
		log.Printf("写入响应失败: %v", err)
	}
}

// 删除文件处理函数
func deleteFile(w http.ResponseWriter, r *http.Request) {
	// 验证权限
	user, err := getUserFromRequest(r)
	if err != nil {
		handleError(w, "认证失败", err, http.StatusUnauthorized)
		return
	}

	// 获取文件ID
	vars := mux.Vars(r)
	fileID := vars["id"]
	if fileID == "" {
		handleError(w, "缺少文件ID", nil, http.StatusBadRequest)
		return
	}

	// 检查权限
	hasPermission, err := authorizer.CheckPermission(r.Context(), user, auth.DeletePermission, "files")
	if err != nil || !hasPermission {
		handleError(w, "权限不足", err, http.StatusForbidden)
		return
	}

	if err := ensureFileService().Delete(r.Context(), fileID); err != nil {
		if errors.Is(err, files.ErrNotFound) {
			handleError(w, "文件不存在", err, http.StatusNotFound)
			return
		}
		handleError(w, "删除文件失败", err, http.StatusInternalServerError)
		return
	}

	// 返回成功响应
	resp := map[string]interface{}{
		"success": true,
		"message": "文件删除成功",
	}

	respondJSON(w, resp, http.StatusOK)
}

// 获取文件状态处理函数
func getFileStatus(w http.ResponseWriter, r *http.Request) {
	// 验证权限
	user, err := getUserFromRequest(r)
	if err != nil {
		handleError(w, "认证失败", err, http.StatusUnauthorized)
		return
	}

	// 获取文件ID
	vars := mux.Vars(r)
	fileID := vars["id"]
	if fileID == "" {
		handleError(w, "缺少文件ID", nil, http.StatusBadRequest)
		return
	}

	// 检查权限
	hasPermission, err := authorizer.CheckPermission(r.Context(), user, auth.ReadPermission, "files")
	if err != nil || !hasPermission {
		handleError(w, "权限不足", err, http.StatusForbidden)
		return
	}

	file, err := ensureFileService().Status(r.Context(), fileID)
	if err != nil {
		if errors.Is(err, files.ErrNotFound) {
			handleError(w, "文件不存在", err, http.StatusNotFound)
			return
		}
		handleError(w, "获取文件状态失败", err, http.StatusInternalServerError)
		return
	}

	fileStatus := map[string]interface{}{
		"file_id":      file.ID,
		"id":           file.ID,
		"filename":     file.Name,
		"name":         file.Name,
		"size":         file.Size,
		"checksum":     file.Checksum,
		"content_type": file.ContentType,
		"status":       file.Status,
		"created_at":   file.CreatedAt.Format(time.RFC3339),
		"metadata": map[string]string{
			"content_type": file.ContentType,
			"created_by":   user.Username,
		},
	}

	respondJSON(w, fileStatus, http.StatusOK)
}

func listFiles(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromRequest(r)
	if err != nil {
		handleError(w, "认证失败", err, http.StatusUnauthorized)
		return
	}

	hasPermission, err := authorizer.CheckPermission(r.Context(), user, auth.ReadPermission, "files")
	if err != nil || !hasPermission {
		handleError(w, "权限不足", err, http.StatusForbidden)
		return
	}

	limit := parsePositiveInt(r.URL.Query().Get("limit"), 20)
	if limit > 100 {
		limit = 100
	}
	offset := parsePositiveInt(r.URL.Query().Get("offset"), 0)

	listed, err := ensureFileService().List(r.Context(), limit, offset)
	if err != nil {
		handleError(w, "列出文件失败", err, http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]interface{}{
		"files":   listed,
		"limit":   limit,
		"offset":  offset,
		"success": true,
	}, http.StatusOK)
}

// 登录处理函数
func login(w http.ResponseWriter, r *http.Request) {
	// 解析请求体
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
		handleError(w, "解析请求失败", err, http.StatusBadRequest)
		return
	}

	// 验证凭证
	user, err := authenticator.Authenticate(r.Context(), credentials.Username, credentials.Password)
	if err != nil {
		handleError(w, "认证失败", err, http.StatusUnauthorized)
		return
	}

	// 生成令牌
	token, err := authenticator.GenerateToken(r.Context(), user)
	if err != nil {
		handleError(w, "生成令牌失败", err, http.StatusInternalServerError)
		return
	}

	// 返回令牌
	resp := map[string]interface{}{
		"token":   token,
		"user_id": user.ID,
		"role":    user.Role,
	}

	respondJSON(w, resp, http.StatusOK)
}

// 刷新令牌处理函数
func refreshToken(w http.ResponseWriter, r *http.Request) {
	// 获取令牌
	token := extractToken(r)
	if token == "" {
		handleError(w, "缺少令牌", nil, http.StatusUnauthorized)
		return
	}

	// 刷新令牌
	newToken, err := authenticator.RefreshToken(r.Context(), token)
	if err != nil {
		handleError(w, "刷新令牌失败", err, http.StatusUnauthorized)
		return
	}

	// 返回新令牌
	resp := map[string]interface{}{
		"token": newToken,
	}

	respondJSON(w, resp, http.StatusOK)
}

// 健康检查处理函数
func healthCheck(w http.ResponseWriter, r *http.Request) {
	// TODO: 实现更复杂的健康检查逻辑
	status := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
	}

	respondJSON(w, status, http.StatusOK)
}

// 从请求中获取用户
func getUserFromRequest(r *http.Request) (*auth.User, error) {
	// 提取令牌
	token := extractToken(r)
	if token == "" {
		return nil, auth.ErrInvalidToken
	}

	// 验证令牌
	return authenticator.ValidateToken(r.Context(), token)
}

// 提取令牌
func extractToken(r *http.Request) string {
	// 从Authorization头中提取
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// Bearer Token
		if strings.HasPrefix(authHeader, "Bearer ") {
			return authHeader[7:]
		}
		return authHeader
	}

	// 从查询参数中提取
	return r.URL.Query().Get("token")
}

// 返回JSON响应
func respondJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("写入JSON响应失败: %v", err)
	}
}

// 处理错误
func handleError(w http.ResponseWriter, message string, err error, statusCode int) {
	errMsg := message
	if err != nil {
		log.Printf("%s: %v", message, err)
		errMsg = fmt.Sprintf("%s: %v", message, err)
	}

	resp := map[string]interface{}{
		"success": false,
		"error":   errMsg,
	}

	respondJSON(w, resp, statusCode)
}

func ensureFileService() *files.Service {
	if fileService == nil {
		initFileService("data/files")
	}
	return fileService
}

func parsePositiveInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}
