package handler

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/dto"
	"cfiber/internal/service"
	"cfiber/pkg/authentication"
	"cfiber/pkg/response"
)

// ProfileHandler 是本人档案端点的 HTTP 适配层。
type ProfileHandler struct {
	svc *service.ProfileService
}

// NewProfileHandler 构造 ProfileHandler。
func NewProfileHandler(svc *service.ProfileService) *ProfileHandler {
	return &ProfileHandler{svc: svc}
}

// GetProfile 处理 GET /v1/users/me。
//
// @Summary      读取本人档案
// @Description  获取当前登录用户的完整个人档案。
// @Tags         个人资料
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} dto.ProfileResource "本人档案"
// @x-order      1
// @Router       /users/me [get]
func (h *ProfileHandler) GetProfile(c fiber.Ctx) error {
	identity, err := authentication.RequireIdentity(c.Context())
	if err != nil {
		return err
	}
	userID := identity.UserID

	profile, err := h.svc.Get(c.Context(), userID)
	if err != nil {
		return err
	}
	return response.OK(c, profile)
}

// UpdateProfile 处理 PATCH /v1/users/me。
//
// @Summary      更新本人档案
// @Description  局部更新个人资料，支持更新昵称、头像、用户名、展示名与手机号。
// @Tags         个人资料
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body dto.UpdateProfileBody true "待更新字段，可只提交其中一部分"
// @Success      200 {object} dto.ProfileResource "更新后的档案"
// @x-order      2
// @Router       /users/me [patch]
func (h *ProfileHandler) UpdateProfile(c fiber.Ctx) error {
	identity, err := authentication.RequireIdentity(c.Context())
	if err != nil {
		return err
	}
	userID := identity.UserID

	var body dto.UpdateProfileBody
	if err := c.Bind().JSON(&body); err != nil {
		return err
	}

	profile, err := h.svc.Update(c.Context(), userID, body.FieldSet())
	if err != nil {
		return err
	}
	return response.OK(c, profile)
}

// UploadAvatar 处理 POST /v1/users/me/avatar。
//
// @Summary      上传本人头像
// @Description  上传并更新当前登录用户的个人头像。仅允许上传不超过 2MB 的 JPEG、PNG 或 WebP 格式图片，自动按魔数头校验真实图像并清除历史旧头像。
// @Tags         个人资料
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        avatar formData file true "头像图片文件 (最大 2MB，支持 JPEG/PNG/WebP)"
// @Success      200 {object} dto.ProfileResource "更新头像后的本人档案"
// @Failure      401 {object} errs.Response "未认证"
// @Failure      422 {object} errs.Response "文件校验失败"
// @x-order      3
// @Router       /users/me/avatar [post]
func (h *ProfileHandler) UploadAvatar(c fiber.Ctx) error {
	identity, err := authentication.RequireIdentity(c.Context())
	if err != nil {
		return err
	}
	userID := identity.UserID

	var body dto.UploadAvatarBody
	if err := c.Bind().Form(&body); err != nil {
		return err
	}
	if body.Avatar == nil {
		file, err := c.FormFile("avatar")
		if err == nil {
			body.Avatar = file
		}
	}

	profile, err := h.svc.UploadAvatar(c.Context(), userID, body.Avatar)
	if err != nil {
		return err
	}

	return response.OK(c, profile)
}
