package v1

import (
	"time"

	"cfiber/internal/database/sqlc"
	"cfiber/internal/enum"
	mediav1 "cfiber/internal/feature/client/media/v1"
	appuser "cfiber/internal/user"
	pkgmedia "cfiber/pkg/media"
)

// UserResource 是认证端点与客户端用户接口共用的用户档案资源视图。
type UserResource struct {
	// 用户唯一标识
	ID string `json:"id" example:"usr_01J9X4N8Y0ABCD"`
	// 用户展示昵称
	Name string `json:"name" example:"Alex Mercer"`
	// 用户电子邮箱
	Email string `json:"email" example:"alex@example.com"`
	// 邮箱是否已完成验证
	EmailVerified bool `json:"emailVerified" example:"true"`
	// 用户头像复合对象（包含相对存储路径与 Imgproxy 防篡改签名访问 URL）
	Avatar *mediav1.ImageVariantResource `json:"avatar"`
	// 用户权限角色 (user: 普通用户, admin: 系统管理员)
	Role enum.Role `json:"role" example:"user" enums:"user,admin"`
	// 唯一登录用户名
	Username *string `json:"username" example:"alexmercer"`
	// 用户显示名称
	DisplayUsername *string `json:"displayUsername" example:"Alex Mercer"`
	// E.164 格式手机号
	PhoneNumber *string `json:"phoneNumber" example:"+8613800138000"`
	// 账号创建时间 (ISO 8601 UTC)
	CreatedAt time.Time `json:"createdAt" example:"2026-09-27T10:00:00Z"`
	// 账号更新时间 (ISO 8601 UTC)
	UpdatedAt time.Time `json:"updatedAt" example:"2026-09-27T10:00:00Z"`
}

// ProjectUser 把用户模型投影为用户档案资源视图。
func ProjectUser(u *sqlc.User, signer ...pkgmedia.Signer) UserResource {
	profile := appuser.ProjectProfile(u)
	return UserResource{
		ID:              profile.ID,
		Name:            profile.Name,
		Email:           profile.Email,
		EmailVerified:   profile.EmailVerified,
		Avatar:          mediav1.BuildAvatar(profile.AvatarPath, signer...),
		Role:            profile.Role,
		Username:        profile.Username,
		DisplayUsername: profile.DisplayUsername,
		PhoneNumber:     profile.PhoneNumber,
		CreatedAt:       profile.CreatedAt,
		UpdatedAt:       profile.UpdatedAt,
	}
}
