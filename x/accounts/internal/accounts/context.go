package accounts

import (
	"context"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/store"
	"cosmossdk.io/x/accounts/internal/prefixstore"
	proto "github.com/cosmos/gogoproto/proto"
)

type (
	underlyingContextKey struct{}
	storeKey             struct{}
	selfKey              struct{}
	fromKey              struct{}
)

func MakeBuildDependencies(invoke func(ctx context.Context, from []byte, to []byte, msg proto.Message) (proto.Message, error)) *BuildDependencies {
	return &BuildDependencies{
		SchemaBuilder: collections.NewSchemaBuilder(StoreService()),
		Execute: func(ctx context.Context, to []byte, msg proto.Message) (proto.Message, error) {
			sender := whoami(ctx)
			return invoke(ctx, sender[:], to[:], msg)
		},
	}
}

// MakeContext will create an isolated execution context for the account with the given address.
func MakeContext(ctx context.Context, svc store.KVStoreService, fromAddr []byte, selfAddr []byte) context.Context {
	ctx = context.WithValue(ctx, underlyingContextKey{}, ctx)
	ctx = context.WithValue(ctx, storeKey{}, prefixstore.NewStore(svc.OpenKVStore(ctx), selfAddr[:]))
	ctx = context.WithValue(ctx, selfKey{}, selfAddr)
	ctx = context.WithValue(ctx, fromKey{}, fromAddr)
	return ctx
}

// GetOriginalContext returns the original context.
func GetOriginalContext(ctx context.Context) context.Context {
	return ctx.Value(underlyingContextKey{}).(context.Context)
}

func StoreService() store.KVStoreService { return storeSvc{} }

type storeSvc struct{}

func (s storeSvc) OpenKVStore(ctx context.Context) store.KVStore {
	return ctx.Value(storeKey{}).(store.KVStore)
}

func whoami(ctx context.Context) [32]byte {
	return ctx.Value(selfKey{}).([32]byte)
}
