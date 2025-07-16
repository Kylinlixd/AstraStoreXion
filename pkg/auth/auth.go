package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// 错误定义
var (
	ErrInvalidCredentials = errors.New("无效的凭证")
	ErrInvalidToken       = errors.New("无效的令牌")
	ErrExpiredToken       = errors.New("令牌已过期")
	ErrPermissionDenied   = errors.New("权限不足")
	ErrUserNotFound       = errors.New("用户不存在")
)

// Role 用户角色
type Role string

const (
	// 管理员角色
	AdminRole Role = "admin"
	// 操作员角色
	OperatorRole Role = "operator"
	// 用户角色
	UserRole Role = "user"
	// 访客角色
	GuestRole Role = "guest"
)

// Permission 权限
type Permission string

const (
	// 读取权限
	ReadPermission Permission = "read"
	// 写入权限
	WritePermission Permission = "write"
	// 删除权限
	DeletePermission Permission = "delete"
	// 管理权限
	AdminPermission Permission = "admin"
)

// User 用户信息
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Authenticator 认证器接口
type Authenticator interface {
	// 验证用户凭证
	Authenticate(ctx context.Context, username, password string) (*User, error)

	// 生成令牌
	GenerateToken(ctx context.Context, user *User) (string, error)

	// 验证令牌
	ValidateToken(ctx context.Context, token string) (*User, error)

	// 刷新令牌
	RefreshToken(ctx context.Context, token string) (string, error)
}

// Authorizer 授权器接口
type Authorizer interface {
	// 检查权限
	CheckPermission(ctx context.Context, user *User, permission Permission, resource string) (bool, error)

	// 获取用户权限
	GetPermissions(ctx context.Context, user *User) (map[string][]Permission, error)

	// 授予权限
	GrantPermission(ctx context.Context, user *User, permission Permission, resource string) error

	// 撤销权限
	RevokePermission(ctx context.Context, user *User, permission Permission, resource string) error
}

// AuthConfig 认证配置
type AuthConfig struct {
	SecretKey     string        // 密钥
	TokenExpiry   time.Duration // 令牌过期时间
	RefreshExpiry time.Duration // 刷新令牌过期时间
	TokenIssuer   string        // 令牌颁发者
	TokenAudience string        // 令牌受众
}

// JWTAuthenticator JWT认证器实现
type JWTAuthenticator struct {
	config AuthConfig
	userDB map[string]*User // 简化实现，实际应使用数据库
}

// NewJWTAuthenticator 创建JWT认证器
func NewJWTAuthenticator(config AuthConfig) *JWTAuthenticator {
	// 设置默认值
	if config.TokenExpiry == 0 {
		config.TokenExpiry = 24 * time.Hour
	}
	if config.RefreshExpiry == 0 {
		config.RefreshExpiry = 7 * 24 * time.Hour
	}

	return &JWTAuthenticator{
		config: config,
		userDB: make(map[string]*User),
	}
}

// RegisterUser 注册用户（简化实现）
func (a *JWTAuthenticator) RegisterUser(user *User, password string) error {
	// 检查用户是否已存在
	if _, exists := a.userDB[user.Username]; exists {
		return fmt.Errorf("用户 %s 已存在", user.Username)
	}

	// 实际应对密码进行哈希处理
	a.userDB[user.Username] = user
	return nil
}

// Authenticate 验证用户凭证
func (a *JWTAuthenticator) Authenticate(ctx context.Context, username, password string) (*User, error) {
	// 查找用户
	user, exists := a.userDB[username]
	if !exists {
		return nil, ErrUserNotFound
	}

	// 实际应验证密码哈希
	// 这里简化实现，假设验证通过

	return user, nil
}

// GenerateToken 生成令牌
func (a *JWTAuthenticator) GenerateToken(ctx context.Context, user *User) (string, error) {
	now := time.Now()

	// 创建声明
	claims := jwt.MapClaims{
		"sub":      user.ID,
		"iss":      a.config.TokenIssuer,
		"aud":      a.config.TokenAudience,
		"exp":      now.Add(a.config.TokenExpiry).Unix(),
		"iat":      now.Unix(),
		"nbf":      now.Unix(),
		"username": user.Username,
		"email":    user.Email,
		"role":     string(user.Role),
	}

	// 创建令牌
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// 签名令牌
	tokenString, err := token.SignedString([]byte(a.config.SecretKey))
	if err != nil {
		return "", fmt.Errorf("签名令牌失败: %v", err)
	}

	return tokenString, nil
}

// ValidateToken 验证令牌
func (a *JWTAuthenticator) ValidateToken(ctx context.Context, tokenString string) (*User, error) {
	// 解析令牌
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// 验证签名方法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("意外的签名方法: %v", token.Header["alg"])
		}

		return []byte(a.config.SecretKey), nil
	})

	if err != nil {
		if ve, ok := err.(*jwt.ValidationError); ok {
			if ve.Errors&jwt.ValidationErrorExpired != 0 {
				return nil, ErrExpiredToken
			}
		}
		return nil, ErrInvalidToken
	}

	// 验证令牌有效性
	if !token.Valid {
		return nil, ErrInvalidToken
	}

	// 获取声明
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	// 创建用户
	user := &User{
		ID:       claims["sub"].(string),
		Username: claims["username"].(string),
		Email:    claims["email"].(string),
		Role:     Role(claims["role"].(string)),
	}

	return user, nil
}

// RefreshToken 刷新令牌
func (a *JWTAuthenticator) RefreshToken(ctx context.Context, tokenString string) (string, error) {
	// 验证旧令牌
	user, err := a.ValidateToken(ctx, tokenString)
	if err != nil && err != ErrExpiredToken {
		return "", err
	}

	// 生成新令牌
	return a.GenerateToken(ctx, user)
}

// SimpleAuthorizer 简单授权器实现
type SimpleAuthorizer struct {
	permissions map[string]map[string][]Permission // 用户ID -> 资源 -> 权限列表
}

// NewSimpleAuthorizer 创建简单授权器
func NewSimpleAuthorizer() *SimpleAuthorizer {
	return &SimpleAuthorizer{
		permissions: make(map[string]map[string][]Permission),
	}
}

// CheckPermission 检查权限
func (a *SimpleAuthorizer) CheckPermission(ctx context.Context, user *User, permission Permission, resource string) (bool, error) {
	// 管理员拥有所有权限
	if user.Role == AdminRole {
		return true, nil
	}

	// 检查用户权限
	if userPerms, exists := a.permissions[user.ID]; exists {
		if resourcePerms, exists := userPerms[resource]; exists {
			for _, p := range resourcePerms {
				if p == permission || p == AdminPermission {
					return true, nil
				}
			}
		}
	}

	// 根据角色检查默认权限
	switch user.Role {
	case OperatorRole:
		// 操作员可以读写，但不能删除
		if permission == ReadPermission || permission == WritePermission {
			return true, nil
		}
	case UserRole:
		// 普通用户只能读
		if permission == ReadPermission {
			return true, nil
		}
	case GuestRole:
		// 访客只能读特定资源
		if permission == ReadPermission && (resource == "public" || resource == "docs") {
			return true, nil
		}
	}

	return false, nil
}

// GetPermissions 获取用户权限
func (a *SimpleAuthorizer) GetPermissions(ctx context.Context, user *User) (map[string][]Permission, error) {
	// 返回用户权限
	if perms, exists := a.permissions[user.ID]; exists {
		return perms, nil
	}

	return make(map[string][]Permission), nil
}

// GrantPermission 授予权限
func (a *SimpleAuthorizer) GrantPermission(ctx context.Context, user *User, permission Permission, resource string) error {
	// 确保用户权限映射存在
	if _, exists := a.permissions[user.ID]; !exists {
		a.permissions[user.ID] = make(map[string][]Permission)
	}

	// 确保资源权限列表存在
	if _, exists := a.permissions[user.ID][resource]; !exists {
		a.permissions[user.ID][resource] = make([]Permission, 0)
	}

	// 检查权限是否已存在
	for _, p := range a.permissions[user.ID][resource] {
		if p == permission {
			return nil // 权限已存在
		}
	}

	// 添加权限
	a.permissions[user.ID][resource] = append(a.permissions[user.ID][resource], permission)
	return nil
}

// RevokePermission 撤销权限
func (a *SimpleAuthorizer) RevokePermission(ctx context.Context, user *User, permission Permission, resource string) error {
	// 检查用户权限映射是否存在
	if _, exists := a.permissions[user.ID]; !exists {
		return nil // 用户没有权限
	}

	// 检查资源权限列表是否存在
	if _, exists := a.permissions[user.ID][resource]; !exists {
		return nil // 资源没有权限
	}

	// 查找并移除权限
	perms := a.permissions[user.ID][resource]
	for i, p := range perms {
		if p == permission {
			// 移除权限
			a.permissions[user.ID][resource] = append(perms[:i], perms[i+1:]...)
			break
		}
	}

	return nil
}
