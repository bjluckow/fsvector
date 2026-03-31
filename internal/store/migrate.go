package store

import (
	"context"
	"fmt"
	"strings"

	_ "embed"

	"github.com/jackc/pgx/v5"
)

//go:embed sql/schema.sql
var schemaTmpl string

func Migrate(ctx context.Context, conn *pgx.Conn, textDim, imageDim int) error {
	schema := strings.ReplaceAll(schemaTmpl, "%%TEXT_DIM%%",
		fmt.Sprintf("%d", textDim))
	schema = strings.ReplaceAll(schema, "%%IMAGE_DIM%%",
		fmt.Sprintf("%d", imageDim))
	_, err := conn.Exec(ctx, schema)
	return err
}
