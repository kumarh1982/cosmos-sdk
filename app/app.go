package app

import (
	"bytes"
	"fmt"
	"strings"

	abci "github.com/tendermint/abci/types"
	cmn "github.com/tendermint/tmlibs/common"
	"github.com/tendermint/tmlibs/log"

	sdk "github.com/cosmos/cosmos-sdk"
	"github.com/cosmos/cosmos-sdk/errors"
	"github.com/cosmos/cosmos-sdk/stack"
	sm "github.com/cosmos/cosmos-sdk/state"
	"github.com/cosmos/cosmos-sdk/version"
)

//nolint
const (
	ModuleNameBase = "base"
	ChainKey       = "chain_id"
)

// Basecoin - The ABCI application
type Basecoin struct {
	*IAVLApp
	handler sdk.Handler
}

var _ abci.Application = &Basecoin{}

// NewBasecoin - create a new instance of the basecoin application
func NewBasecoin(handler sdk.Handler, store *Store, logger log.Logger) Basecoin {
	return Basecoin{
		handler: handler,
		IAVLApp: NewIAVLApp(store, logger),
	}
}

// InitState - used to setup state (was SetOption)
// to be used by InitChain later
func (app Basecoin) InitState(key string, value string) string {

	module, key := splitKey(key)
	state := app.state.Append()

	if module == ModuleNameBase {
		if key == ChainKey {
			app.info.SetChainID(state, value)
			return "Success"
		}
		return fmt.Sprintf("Error: unknown base option: %s", key)
	}

	log, err := app.handler.InitState(app.logger, state, module, key, value)
	if err == nil {
		return log
	}
	return "Error: " + err.Error()
}

// DeliverTx - ABCI
func (app Basecoin) DeliverTx(txBytes []byte) abci.Result {
	tx, err := sdk.LoadTx(txBytes)
	if err != nil {
		return errors.Result(err)
	}

	ctx := stack.NewContext(
		app.GetChainID(),
		app.height,
		app.logger.With("call", "delivertx"),
	)
	res, err := app.handler.DeliverTx(ctx, app.state.Append(), tx)

	if err != nil {
		return errors.Result(err)
	}
	app.addValChange(res.Diff)
	return sdk.ToABCI(res)
}

// CheckTx - ABCI
func (app Basecoin) CheckTx(txBytes []byte) abci.Result {
	tx, err := sdk.LoadTx(txBytes)
	if err != nil {
		return errors.Result(err)
	}

	ctx := stack.NewContext(
		app.GetChainID(),
		app.height,
		app.logger.With("call", "checktx"),
	)
	res, err := app.handler.CheckTx(ctx, app.state.Check(), tx)

	if err != nil {
		return errors.Result(err)
	}
	return sdk.ToABCI(res)
}

// IAVLApp is a generic app backed by an IAVL tree
// You can embed it and add your own DeliverTx/CheckTx logic
type IAVLApp struct {
	info  *sm.ChainState
	state *Store

	pending []*abci.Validator
	height  uint64
	logger  log.Logger
}

// NewIAVLApp creates an instance of an IAVLApp
func NewIAVLApp(store *Store, logger log.Logger) *IAVLApp {
	return &IAVLApp{
		info:   sm.NewChainState(),
		state:  store,
		logger: logger,
	}
}

// GetChainID returns the currently stored chain
func (app *IAVLApp) GetChainID() string {
	return app.info.GetChainID(app.state.Committed())
}

// GetState is back... please kill me
func (app *IAVLApp) GetState() sm.SimpleDB {
	return app.state.Append()
}

// Info - ABCI
func (app *IAVLApp) Info() abci.ResponseInfo {
	resp := app.state.Info()
	app.height = resp.LastBlockHeight
	return abci.ResponseInfo{
		Data:             fmt.Sprintf("Basecoin v%v", version.Version),
		LastBlockHeight:  resp.LastBlockHeight,
		LastBlockAppHash: resp.LastBlockAppHash,
	}
}

// SetOption - ABCI
func (app *IAVLApp) SetOption(key string, value string) string {
	return "Not Implemented"
}

// Query - ABCI
func (app *IAVLApp) Query(reqQuery abci.RequestQuery) (resQuery abci.ResponseQuery) {
	if len(reqQuery.Data) == 0 {
		resQuery.Log = "Query cannot be zero length"
		resQuery.Code = abci.CodeType_EncodingError
		return
	}

	return app.state.Query(reqQuery)
}

// Commit - ABCI
func (app *IAVLApp) Commit() (res abci.Result) {
	// Commit state
	res = app.state.Commit()
	if res.IsErr() {
		cmn.PanicSanity("Error getting hash: " + res.Error())
	}
	return res
}

// InitChain - ABCI
func (app *IAVLApp) InitChain(validators []*abci.Validator) {
}

// BeginBlock - ABCI
func (app *IAVLApp) BeginBlock(hash []byte, header *abci.Header) {
	app.height++
	// TODO: trigger ticks on modules
}

// EndBlock - ABCI
// Returns a list of all validator changes made in this block
func (app *IAVLApp) EndBlock(height uint64) (res abci.ResponseEndBlock) {
	// TODO: cleanup in case a validator exists multiple times in the list
	res.Diffs = app.pending
	app.pending = nil
	return
}

func (app *IAVLApp) addValChange(diffs []*abci.Validator) {
	for _, d := range diffs {
		idx := pubKeyIndex(d, app.pending)
		if idx >= 0 {
			app.pending[idx] = d
		} else {
			app.pending = append(app.pending, d)
		}
	}
}

// return index of list with validator of same PubKey, or -1 if no match
func pubKeyIndex(val *abci.Validator, list []*abci.Validator) int {
	for i, v := range list {
		if bytes.Equal(val.PubKey, v.PubKey) {
			return i
		}
	}
	return -1
}

//TODO move split key to tmlibs?

// Splits the string at the first '/'.
// if there are none, assign default module ("base").
func splitKey(key string) (string, string) {
	if strings.Contains(key, "/") {
		keyParts := strings.SplitN(key, "/", 2)
		return keyParts[0], keyParts[1]
	}
	return ModuleNameBase, key
}
