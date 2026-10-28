package adminhandler

import (
	"io"

	"github.com/gofiber/fiber/v3"

	admindto "cfiber/internal/dto/admin"
	adminservice "cfiber/internal/service/admin"
	"cfiber/pkg/response"
)

// UserHandler 是管理端用户 CRUD 的 HTTP 适配层。
type UserHandler struct {
	svc *adminservice.UserService
}

// NewUserHandler 构造 UserHandler。
func NewUserHandler(svc *adminservice.UserService) *UserHandler {
	return &UserHandler{svc: svc}
}

// ListUsers 处理 GET /v1/admin/users。
//
// @Summary      用户分页列表
// @Description  根据分页、关键字、状态与角色条件查询系统用户列表。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        query query admindto.ListUsersQueryParams false "查询与分页参数"
// @Success      200 {object} admindto.UserPageResource "用户分页列表"
// @x-order      1
// @Router       /admin/users [get]
func (h *UserHandler) ListUsers(c fiber.Ctx) error {
	var query admindto.ListUsersQueryParams
	if err := c.Bind().Query(&query); err != nil {
		return err
	}

	items, meta, err := h.svc.ListUsers(c.Context(), query)
	if err != nil {
		return err
	}
	return response.Paginated(c, items, meta)
}

// ExportUsers 处理 GET /v1/admin/users/export。
//
// @Summary      导出系统用户
// @Description  根据关键字、状态与角色条件分块流式导出用户列表为 Excel 文件，免除内存溢出风险。
// @Description  导出固定按用户 ID 升序逐行输出（单页成本与已翻页深度无关），与列表页的 sort 参数无关。
// @Tags         用户管理
// @Accept       json
// @Produce      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security     BearerAuth
// @Param        query query admindto.ListUsersQueryParams false "筛选与排序条件"
// @Success      200 {file} binary "用户数据 Excel 文件"
// @x-order      6
// @Router       /admin/users/export [get]
func (h *UserHandler) ExportUsers(c fiber.Ctx) error {
	var query admindto.ListUsersQueryParams
	if err := c.Bind().Query(&query); err != nil {
		return err
	}

	c.Set(fiber.HeaderContentDisposition, `attachment; filename="users.xlsx"`)
	c.Set(fiber.HeaderContentType, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")

	pr, pw := io.Pipe()
	go func() {
		err := h.svc.ExportUsers(c.Context(), query, pw)
		_ = pw.CloseWithError(err)
	}()

	return c.SendStream(pr)
}

// GetUser 处理 GET /v1/admin/users/:id。
//
// @Summary      获取用户详情
// @Description  根据用户唯一标识查询用户完整档案与管理审计信息。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "用户唯一标识" example("0192e2b3-847e-7c5b-9d47-68198f16182f")
// @Success      200 {object} admindto.UserResource "用户完整档案"
// @x-order      2
// @Router       /admin/users/{id} [get]
func (h *UserHandler) GetUser(c fiber.Ctx) error {
	var params admindto.UserPathParams
	if err := c.Bind().URI(&params); err != nil {
		return err
	}

	entity, err := h.svc.GetUser(c.Context(), params.ID)
	if err != nil {
		return err
	}
	return response.OK(c, entity)
}

// CreateUser 处理 POST /v1/admin/users。
//
// @Summary      创建系统用户
// @Description  管理员直接创建新用户账号，指定邮箱、口令、昵称与角色等信息。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body admindto.CreateUserBody true "创建用户载荷"
// @Success      201 {object} admindto.UserResource "新建成功的用户对象"
// @x-order      3
// @Router       /admin/users [post]
func (h *UserHandler) CreateUser(c fiber.Ctx) error {
	var body admindto.CreateUserBody
	if err := c.Bind().JSON(&body); err != nil {
		return err
	}

	created, err := h.svc.CreateUser(c.Context(), body)
	if err != nil {
		return err
	}
	return response.Created(c, created)
}

// UpdateUser 处理 PATCH /v1/admin/users/:id。
//
// @Summary      更新用户信息
// @Description  支持局部字段三态更新，可修改昵称、头像、用户名、手机号、角色、状态及封禁属性。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "用户唯一标识" example("0192e2b3-847e-7c5b-9d47-68198f16182f")
// @Param        payload body admindto.UpdateUserBody true "局部更新载荷"
// @Success      200 {object} admindto.UserResource "更新后的用户对象"
// @x-order      4
// @Router       /admin/users/{id} [patch]
func (h *UserHandler) UpdateUser(c fiber.Ctx) error {
	var params admindto.UserPathParams
	if err := c.Bind().URI(&params); err != nil {
		return err
	}

	var body admindto.UpdateUserBody
	if err := c.Bind().JSON(&body); err != nil {
		return err
	}

	updated, err := h.svc.UpdateUser(c.Context(), params.ID, body)
	if err != nil {
		return err
	}
	return response.OK(c, updated)
}

// DeleteUser 处理 DELETE /v1/admin/users/:id，返回 204。
//
// @Summary      注销删除用户
// @Description  软删除指定用户，保留数据以支持审计与恢复；其关联凭据随账号状态一同失效。
// @Tags         用户管理
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "用户唯一标识" example("0192e2b3-847e-7c5b-9d47-68198f16182f")
// @Success      204 "删除成功，无响应体"
// @x-order      5
// @Router       /admin/users/{id} [delete]
func (h *UserHandler) DeleteUser(c fiber.Ctx) error {
	var params admindto.UserPathParams
	if err := c.Bind().URI(&params); err != nil {
		return err
	}

	if err := h.svc.DeleteUser(c.Context(), params.ID); err != nil {
		return err
	}
	return response.NoContent(c)
}
