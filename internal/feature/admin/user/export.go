package adminuser

import (
	"context"
	"io"
	"time"

	"cfiber/internal/database/sqlc"
	"cfiber/internal/user"
	"cfiber/pkg/excel"
)

const (
	// userSheet 是用户导出文件的工作表名。
	userSheet = "Users"
	// userChunkSize 是单次物理拉取的批量大小。
	userChunkSize = 500
)

// userColumns 定义用户导出的声明式强类型列契约。
var userColumns = []excel.ExportColumn[sqlc.User]{
	{
		Header: "ID",
		Width:  28,
		Encode: func(u *sqlc.User) any { return u.ID },
	},
	{
		Header: "Name",
		Width:  20,
		Encode: func(u *sqlc.User) any { return u.Name },
	},
	{
		Header: "Email",
		Width:  28,
		Encode: func(u *sqlc.User) any { return u.Email },
	},
	{
		Header: "Role",
		Width:  12,
		Align:  excel.AlignCenter,
		Encode: func(u *sqlc.User) any { return u.Role },
	},
	{
		Header: "Status",
		Width:  12,
		Align:  excel.AlignCenter,
		Encode: func(u *sqlc.User) any { return u.Status },
	},
	{
		Header: "Created At",
		Width:  22,
		Align:  excel.AlignCenter,
		Encode: func(u *sqlc.User) any {
			if u.CreatedAt.IsZero() {
				return ""
			}
			return u.CreatedAt.UTC().Format(time.RFC3339)
		},
	},
}

// ExportUsers 把符合条件的用户按键集游标分块拉取并流式写入 xlsx。
func ExportUsers(
	ctx context.Context,
	w io.Writer,
	fetch func(ctx context.Context, filter user.ListFilter, afterID string, limit int) ([]sqlc.User, error),
	filter user.ListFilter,
) error {
	var (
		afterID string
		buffer  []sqlc.User
		cursor  int
		ended   bool
	)

	next := func() (sqlc.User, bool, error) {
		for cursor >= len(buffer) {
			if err := ctx.Err(); err != nil {
				return sqlc.User{}, false, err
			}
			if ended {
				return sqlc.User{}, false, nil
			}

			chunk, err := fetch(ctx, filter, afterID, userChunkSize)
			if err != nil {
				return sqlc.User{}, false, err
			}
			if len(chunk) == 0 {
				return sqlc.User{}, false, nil
			}
			if len(chunk) < userChunkSize {
				ended = true
			}

			buffer = chunk
			cursor = 0
			afterID = chunk[len(chunk)-1].ID
		}

		item := buffer[cursor]
		cursor++
		return item, true, nil
	}

	return excel.WriteTable(ctx, w, userSheet, userColumns, next)
}
