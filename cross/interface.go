package cross

import (
	"context"

	"github.com/simplechain-org/go-simplechain/common"
	"github.com/simplechain-org/go-simplechain/core"
	"github.com/simplechain-org/go-simplechain/core/state"
	"github.com/simplechain-org/go-simplechain/core/types"
	"github.com/simplechain-org/go-simplechain/core/vm"
	"github.com/simplechain-org/go-simplechain/event"
	"github.com/simplechain-org/go-simplechain/rpc"
)

type ctxStore interface {
	// AddRemotes should add the given transactions to the pool.
	AddRemote(*types.CrossTransaction) error
	// AddRemotes should add the given transactions to the pool.
	AddLocal(*types.CrossTransaction) error

	AddCWss([]*types.CrossTransactionWithSignatures) []error

	ValidateCtx(*types.CrossTransaction) error

	RemoveRemotes([]*types.ReceptTransaction) error

	RemoveLocals([]*types.FinishInfo) error

	RemoveFromLocalsByTransaction(common.Hash) error

	SubscribeCWssResultEvent(chan<- core.NewCWsEvent) event.Subscription

	//SubscribeNewCWssEvent(chan<- core.NewCWssEvent) event.Subscription

	ReadFromLocals(common.Hash) *types.CrossTransactionWithSignatures
}

type rtxStore interface {
	AddRemote(*types.ReceptTransaction) error
	//AddRemotes([]*types.ReceptTransaction) []error
	AddLocal(*types.ReceptTransaction) error
	//AddLocals([]*types.ReceptTransaction) []error

	ValidateRtx(rtx *types.ReceptTransaction) error

	//SubscribeNewRtxEvent(chan<- core.NewRTxEvent) event.Subscription
	SubscribeRWssResultEvent(chan<- core.NewRWsEvent) event.Subscription

	SubscribeNewRWssEvent(chan<- core.NewRWssEvent ) event.Subscription

	AddLocals(...*types.ReceptTransactionWithSignatures) []error
	RemoveLocals([]*types.ReceptTransactionWithSignatures) error
	//ReadFromLocals(ctxId common.Hash) *types.ReceptTransactionWithSignatures
	//WriteToLocals(rtws *types.ReceptTransactionWithSignatures) error

	ReadFromLocals(ctxId common.Hash) (*types.ReceptTransactionWithSignatures)
}

type simplechain interface {
	GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error)
	BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Block, error)
	HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error)
	StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *types.Header, error)
}