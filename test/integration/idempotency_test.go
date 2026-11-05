//go:build integration

package integration_test

import (
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/storage/valkey"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/config"
)

// openIdempotencyStorage 打开真实 Valkey Storage，供中间件集成验证使用。
func openIdempotencyStorage(t *testing.T) fiber.Storage {
	t.Helper()

	cfg, err := config.Load()
	require.NoError(t, err, "加载配置失败")

	storage := valkey.New(valkey.Config{
		URL:          cfg.Cache.URL,
		DisableCache: true,
	})
	t.Cleanup(func() { _ = storage.Close() })
	return storage
}

// TestIdempotencyStorageRoundTrip 验证 Fiber Storage 的写入、读取与未命中语义。
func TestIdempotencyStorageRoundTrip(t *testing.T) {
	storage := openIdempotencyStorage(t)
	key := "integration:" + uuid.NewString()

	missing, err := storage.Get(key)
	require.NoError(t, err)
	assert.Nil(t, missing)

	payload := []byte(`{"statusCode":201,"body":"b3JkZXI="}`)
	require.NoError(t, storage.Set(key, payload, 2*time.Second))

	loaded, err := storage.Get(key)
	require.NoError(t, err)
	assert.Equal(t, payload, loaded)

	require.NoError(t, storage.Delete(key))
	deleted, err := storage.Get(key)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}
