// Package route_test 验证前端自助档案模块的对外契约（Q27）。
package route_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/dto"
	"cfiber/internal/enum"
	"cfiber/internal/handler"
	"cfiber/internal/middleware/global"
	"cfiber/internal/service"
	"cfiber/internal/user"
	"cfiber/pkg/authentication"
	"cfiber/pkg/validator"
)

var testValidator *validator.Validator

// TestMain 安装校验器：本包通过 fiber 绑定驱动多部分表单与 JSON。
func TestMain(m *testing.M) {
	var err error
	testValidator, err = validator.New()
	if err != nil {
		panic(err)
	}
	testValidator.Install()

	os.Exit(m.Run())
}

// executeRequest 驱动一次 HTTP 测试并返回状态码和响应体。
func executeRequest(t *testing.T, app *fiber.App, req *http.Request) (int, string) {
	t.Helper()
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

// TestUpdateProfileRequestShape 校验自助更新只暴露 Q27 允许的字段。
func TestUpdateProfileRequestShape(t *testing.T) {
	var request dto.UpdateProfileBody
	require.NoError(t, json.Unmarshal([]byte(`{
		"name":"Alice",
		"avatar_path":null,
		"username":"alice_01",
		"displayUsername":"Alice",
		"phoneNumber":"+8613800138000",
		"email":"should-be-ignored@cfiber.local",
		"role":"admin"
	}`), &request))

	// 邮箱与角色不在自助契约里：JSON 里出现也不会被解析进结构体。
	encoded, err := json.Marshal(request)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "email")
	assert.NotContains(t, string(encoded), "role")

	assert.True(t, request.Name.IsValue())
	assert.True(t, request.AvatarPath.IsNull())
	assert.True(t, request.Username.IsValue())
	assert.True(t, request.DisplayUsername.IsValue())
	assert.True(t, request.PhoneNumber.IsValue())
}

// TestUploadAvatarValidation 校验上传头像的权限要求、字段提取与存在性校验。
func TestUploadAvatarValidation(t *testing.T) {
	storage := &fakeStorage{
		files: map[string]bool{
			"uploads/avatars/valid.png": true,
		},
	}
	users := &fakeUsers{profile: user.Profile{}}
	handler := handler.NewProfileHandler(service.MustNewProfileService(service.ProfileDeps{Users: users, Storage: storage}))

	newTestEngine := func(withAuth bool) *fiber.App {
		app := fiber.New(fiber.Config{
			StructValidator: testValidator,
			ErrorHandler:    global.HandleError,
		})
		app.Use(func(c fiber.Ctx) error {
			if withAuth {
				c.SetContext(authentication.With(c.Context(), authentication.Identity{
					UserID: "user_test_avatar_1",
					Role:   string(enum.RoleUser),
				}))
			}
			return c.Next()
		})
		app.Post("/v1/users/me/avatar", handler.UploadAvatar)
		return app
	}

	t.Run("未登录拒绝 401", func(t *testing.T) {
		app := newTestEngine(false)
		req := createJSONRequest(http.MethodPost, "/v1/users/me/avatar", `{"avatar_path":"uploads/avatars/valid.png"}`)
		code, _ := executeRequest(t, app, req)

		assert.Equal(t, http.StatusUnauthorized, code)
	})

	t.Run("缺失 avatar_path 返回 422", func(t *testing.T) {
		app := newTestEngine(true)
		req := createJSONRequest(http.MethodPost, "/v1/users/me/avatar", `{}`)
		code, body := executeRequest(t, app, req)

		assert.Equal(t, http.StatusUnprocessableEntity, code)
		assert.Contains(t, body, "avatar_path")
	})

	t.Run("存储中不存在的对象路径返回 400", func(t *testing.T) {
		app := newTestEngine(true)
		req := createJSONRequest(http.MethodPost, "/v1/users/me/avatar", `{"avatar_path":"uploads/avatars/non_existent.png"}`)
		code, body := executeRequest(t, app, req)

		assert.Equal(t, http.StatusBadRequest, code)
		assert.Contains(t, body, "does not exist in storage")
	})

	t.Run("有效存储路径更新成功 200", func(t *testing.T) {
		app := newTestEngine(true)
		req := createJSONRequest(http.MethodPost, "/v1/users/me/avatar", `{"avatar_path":"uploads/avatars/valid.png"}`)
		code, body := executeRequest(t, app, req)

		assert.Equal(t, http.StatusOK, code, body)
		assert.True(t, users.updated)
		assert.Equal(t, "uploads/avatars/valid.png", users.gotFields.AvatarPath.MustGet())
	})
}

// TestUploadAvatarWritesAvatarPath 校验合法头像会把相对 Key 写入 user.avatar_path，且作用域为客户端自助。
func TestUploadAvatarWritesAvatarPath(t *testing.T) {
	storage := &fakeStorage{
		files: map[string]bool{
			"uploads/avatars/new.png": true,
		},
	}
	oldPath := "uploads/avatars/old.png"
	users := &fakeUsers{
		profile: user.Profile{AvatarPath: &oldPath},
	}
	app := newAvatarEngine(t, storage, users)

	code, body := executeRequest(t, app, createJSONRequest(http.MethodPost, "/v1/users/me/avatar", `{"avatar_path":"uploads/avatars/new.png"}`))

	require.Equal(t, http.StatusOK, code, body)
	assert.True(t, users.updated)
	assert.Equal(t, user.ScopeClient, users.gotScope)

	avatarPath, ok := users.gotFields.AvatarPath.Get()
	require.True(t, ok)
	assert.Equal(t, "uploads/avatars/new.png", avatarPath)
	assert.Contains(t, storage.deleted, oldPath, "旧头像必须被异步清理")
}

// newAvatarEngine 构造带认证注入与统一错误渲染的头像上传引擎。
func newAvatarEngine(t *testing.T, storage *fakeStorage, users *fakeUsers) *fiber.App {
	t.Helper()

	handler := handler.NewProfileHandler(service.MustNewProfileService(service.ProfileDeps{Users: users, Storage: storage}))
	app := fiber.New(fiber.Config{
		StructValidator: testValidator,
		ErrorHandler:    global.HandleError,
	})
	app.Use(func(c fiber.Ctx) error {
		c.SetContext(authentication.With(c.Context(), authentication.Identity{
			UserID: "user_test_avatar_1",
			Role:   string(enum.RoleUser),
		}))
		return c.Next()
	})
	app.Post("/v1/users/me/avatar", handler.UploadAvatar)
	return app
}

// fakeStorage 模拟对象存储。
type fakeStorage struct {
	files   map[string]bool
	deleted []string
}

func (s *fakeStorage) Exists(_ context.Context, path string) (bool, error) {
	if s.files == nil {
		return false, nil
	}
	return s.files[path], nil
}

func (s *fakeStorage) Delete(_ context.Context, path string) error {
	s.deleted = append(s.deleted, path)
	delete(s.files, path)
	return nil
}

// fakeUsers 记录档案写入入参，便于断言作用域与字段。
type fakeUsers struct {
	profile   user.Profile
	err       error
	updated   bool
	gotFields user.FieldSet
	gotScope  user.Scope
}

func (f *fakeUsers) GetProfile(_ context.Context, _ string) (user.Profile, error) {
	return f.profile, f.err
}

func (f *fakeUsers) UpdateProfile(
	_ context.Context,
	_ string,
	fields user.FieldSet,
	scope user.Scope,
) (user.Profile, error) {
	f.updated = true
	f.gotFields = fields
	f.gotScope = scope
	if f.err != nil {
		return user.Profile{}, f.err
	}
	return f.profile, nil
}

// createJSONRequest 构造 JSON 测试请求。
func createJSONRequest(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
