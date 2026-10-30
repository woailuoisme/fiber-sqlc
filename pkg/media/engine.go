// Package media 提供媒体文件上传、验证、处理、命名策略与 Imgproxy 防篡改签名服务。
package media

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/samber/oops"

	"cfiber/internal/config"
	"cfiber/pkg/errs"
	"cfiber/pkg/storage"
)

// Engine 定义媒体中心统一深度服务契约。
type Engine interface {
	// Presign 校验文件参数并签发 S3 预签名直传凭证
	Presign(ctx context.Context, req PresignRequest) (*PresignResult, error)
	// Sign 生成指定规格的 Imgproxy 防篡改签名访问 URL
	Sign(path string, preset ...Preset) string
	// Variants 为存储相对路径生成全套标准图片衍生变体集
	Variants(path string) *ImageVariants
	// Exists 校验对象在对象存储中是否存在
	Exists(ctx context.Context, path string) (bool, error)
	// Delete 删除对象存储中的指定路径
	Delete(ctx context.Context, path string) error
	// Ping 探针检测 Imgproxy 服务连通性（实现 ready.Pinger 契约）
	Ping(ctx context.Context) error
	// Driver 返回图像处理驱动标识
	Driver() string
	// Image 获取底层 Imgproxy 客户端
	Image() Client
	// Disk 获取底层存储 Disk
	Disk() storage.Disk
}

// engineImpl 实现 Engine 接口。
type engineImpl struct {
	disk      storage.Disk
	image     Client
	path      PathStrategy
	validator Validator
}

var _ Engine = (*engineImpl)(nil)

// NewEngine 构造带有工业级默认装配的统一媒体引擎。
func NewEngine(
	mediaCfg config.MediaConfig,
	storageCfg config.StorageConfig,
	disk storage.Disk,
) Engine {
	imgClient := NewWithStorage(mediaCfg.Imgproxy, storageCfg)

	v := NewValidator()
	if mediaCfg.MaxFileSize > 0 {
		v.WithMaxFileSize(mediaCfg.MaxFileSize)
	}
	if len(mediaCfg.AllowedExtensions) > 0 {
		v.WithAllowedExtensions(mediaCfg.AllowedExtensions...)
	}
	if len(mediaCfg.AllowedMimeTypes) > 0 {
		v.WithAllowedMimeTypes(mediaCfg.AllowedMimeTypes...)
	}

	return &engineImpl{
		disk:      disk,
		image:     imgClient,
		path:      NewCategoryDatePathStrategy("uploads"),
		validator: v,
	}
}

// Presign 校验文件参数并向底层对象存储申请预签名直传 URL。
func (e *engineImpl) Presign(ctx context.Context, req PresignRequest) (*PresignResult, error) {
	if e.disk == nil {
		return nil, oops.Code(errs.CodeInternal).In("media.Engine.Presign").Errorf("storage disk is not configured")
	}

	category := strings.ToLower(strings.TrimSpace(req.Category))
	if category == "" {
		category = "default"
	}

	fileName := strings.TrimSpace(req.FileName)
	if fileName == "" {
		return nil, oops.Code(errs.CodeBadRequest).In("media.Engine.Presign").Errorf("file name cannot be empty")
	}

	if err := e.validator.Validate(ctx, fileName, req.FileSize, req.MimeType); err != nil {
		return nil, oops.Code(errs.CodeBadRequest).In("media.Engine.Presign").Wrap(err)
	}

	ext := resolveExtension(fileName, nil)
	id, err := uuid.NewV7()
	if err != nil {
		id = uuid.New()
	}
	uniqueFileName := id.String() + ext
	targetPath := e.path.GeneratePath(ctx, category, uniqueFileName)

	ttl := DefaultPresignTTL
	uploadURL, err := e.disk.PresignPut(ctx, targetPath, ttl)
	if err != nil {
		return nil, oops.Code(errs.CodeInternal).In("media.Engine.Presign").Wrap(err)
	}

	headers := make(map[string]string)
	if strings.TrimSpace(req.MimeType) != "" {
		headers["Content-Type"] = req.MimeType
	}

	return &PresignResult{
		UploadURL: uploadURL,
		Path:      targetPath,
		Method:    "PUT",
		Headers:   headers,
		ExpiresIn: int64(ttl.Seconds()),
	}, nil
}

// Sign 生成指定规格的 Imgproxy 防篡改签名访问 URL。
func (e *engineImpl) Sign(path string, preset ...Preset) string {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" || e.image == nil {
		return cleanPath
	}

	p := lo.FirstOr(preset, PresetAvatarLarge)
	u, err := e.image.URL(cleanPath, p.Options()...)
	if err != nil {
		return cleanPath
	}
	return u
}

// Variants 为存储相对路径生成全套标准图片衍生变体集。
func (e *engineImpl) Variants(path string) *ImageVariants {
	return BuildImageVariants(e.image, path)
}

// Exists 校验对象在对象存储中是否存在。
func (e *engineImpl) Exists(ctx context.Context, path string) (bool, error) {
	if e.disk == nil {
		return false, nil
	}
	return e.disk.Exists(ctx, path)
}

// Delete 删除对象存储中的指定路径。
func (e *engineImpl) Delete(ctx context.Context, path string) error {
	if e.disk == nil {
		return nil
	}
	return e.disk.Delete(ctx, path)
}

// Ping 执行底层 imgproxy 连通性探测。
func (e *engineImpl) Ping(ctx context.Context) error {
	if e.image == nil {
		return nil
	}
	return e.image.Ping(ctx)
}

// Driver 返回驱动标识。
func (e *engineImpl) Driver() string {
	if e.image != nil {
		return e.image.Driver()
	}
	return "imgproxy"
}

// Image 获取底层 Imgproxy 客户端。
func (e *engineImpl) Image() Client {
	return e.image
}

// Disk 获取底层存储 Disk。
func (e *engineImpl) Disk() storage.Disk {
	return e.disk
}
