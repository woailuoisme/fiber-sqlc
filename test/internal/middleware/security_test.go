package middleware_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/middleware/global"
)

func TestEnforceJSONContentType(t *testing.T) {
	m := global.New(global.Deps{})

	newApp := func() *fiber.App {
		app := fiber.New(fiber.Config{
			ErrorHandler: global.HandleError,
		})
		app.Use(m.EnforceJSONContentType())
		app.Post("/test", func(c fiber.Ctx) error {
			return c.SendStatus(http.StatusOK)
		})
		return app
	}

	t.Run("Valid JSON passes", func(t *testing.T) {
		app := newApp()
		body := []byte(`{"key":"val"}`)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Valid JSON with charset passes", func(t *testing.T) {
		app := newApp()
		body := []byte(`{"key":"val"}`)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Empty body passes", func(t *testing.T) {
		app := newApp()
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Multipart form passes", func(t *testing.T) {
		app := newApp()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		require.NoError(t, writer.WriteField("field", "value"))
		require.NoError(t, writer.Close())

		req := httptest.NewRequest(http.MethodPost, "/test", &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("Invalid text/plain fails", func(t *testing.T) {
		app := newApp()
		body := []byte(`plain text`)
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(body))
		req.Header.Set("Content-Type", "text/plain")
		resp, err := app.Test(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}
