package pause

import (
	"context"

	"github.com/sagernet/sing/service"
)

func ManagerFromContext(ctx context.Context) Manager {
	return service.FromContext[Manager](ctx)
}

func ContextWithManager(ctx context.Context, manager Manager) context.Context {
	return service.ContextWith[Manager](ctx, manager)
}

func ContextWithDefaultManager(ctx context.Context) context.Context {
	if service.FromContext[Manager](ctx) != nil {
		return ctx
	}
	return service.ContextWith[Manager](ctx, NewDefaultManager(ctx))
}
