// internal/auth/context.go
package auth

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey struct{}

// ContextWithUserID кладёт ID аутентифицированного пользователя в контекст.
func ContextWithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

// UserIDFromContext достаёт ID пользователя (его кладёт auth-интерсептор).
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(ctxKey{}).(uuid.UUID)
	return id, ok
}