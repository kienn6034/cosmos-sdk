package accounts

import (
	"context"

	"cosmossdk.io/collections"
	"github.com/cosmos/gogoproto/proto"
)

// Account defines a generic account interface.
type Account interface {
	// RegisterInitHandler must be used by the account to register its initialisation handler.
	RegisterInitHandler(router *InitRouter) error
	// RegisterExecuteHandlers is given a router and the account should register
	// its execute handlers.
	RegisterExecuteHandlers(router *ExecuteRouter) error
	// RegisterQueryHandlers is given a router and the account should register
	// its query handlers.
	RegisterQueryHandlers(router *QueryRouter) error
}

type BuildDependencies struct {
	SchemaBuilder *collections.SchemaBuilder
	Execute       func(ctx context.Context, target []byte, msg proto.Message) (proto.Message, error)
	Query         func(ctx context.Context, target []byte, msg proto.Message) (proto.Message, error)
}
