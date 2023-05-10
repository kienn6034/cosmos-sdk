package keeper_test

import (
	"math/big"
	"testing"

	"cosmossdk.io/log"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"gotest.tools/v3/assert"

	cmtprototypes "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil/integration"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/distribution"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	distrtypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/slashing"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/cosmos/cosmos-sdk/x/staking/testutil"
	"github.com/cosmos/cosmos-sdk/x/staking/types"
)

var PKs = simtestutil.CreateTestPubKeys(500)

type fixture struct {
	app *integration.App

	sdkCtx sdk.Context
	cdc    codec.Codec
	keys   map[string]*storetypes.KVStoreKey

	accountKeeper authkeeper.AccountKeeper
	bankKeeper    bankkeeper.Keeper
	stakingKeeper *stakingkeeper.Keeper
}

func init() {
	sdk.DefaultPowerReduction = math.NewIntFromBigInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

// intended to be used with require/assert:  require.True(ValEq(...))
func ValEq(t *testing.T, exp, got types.Validator) (*testing.T, bool, string, types.Validator, types.Validator) {
	return t, exp.MinEqual(&got), "expected:\n%v\ngot:\n%v", exp, got
}

// generateAddresses generates numAddrs of normal AccAddrs and ValAddrs
func generateAddresses(f *fixture, numAddrs int) ([]sdk.AccAddress, []sdk.ValAddress) {
	addrDels := simtestutil.AddTestAddrsIncremental(f.bankKeeper, f.stakingKeeper, f.sdkCtx, numAddrs, sdk.NewInt(10000))
	addrVals := simtestutil.ConvertAddrsToValAddrs(addrDels)

	return addrDels, addrVals
}

func createValidators(t *testing.T, f *fixture, powers []int64) ([]sdk.AccAddress, []sdk.ValAddress, []types.Validator) {
	addrs := simtestutil.AddTestAddrsIncremental(f.bankKeeper, f.stakingKeeper, f.sdkCtx, 5, f.stakingKeeper.TokensFromConsensusPower(f.sdkCtx, 300))
	valAddrs := simtestutil.ConvertAddrsToValAddrs(addrs)
	pks := simtestutil.CreateTestPubKeys(5)

	val1 := testutil.NewValidator(t, valAddrs[0], pks[0])
	val2 := testutil.NewValidator(t, valAddrs[1], pks[1])
	vals := []types.Validator{val1, val2}

	f.stakingKeeper.SetValidator(f.sdkCtx, val1)
	f.stakingKeeper.SetValidator(f.sdkCtx, val2)
	f.stakingKeeper.SetValidatorByConsAddr(f.sdkCtx, val1)
	f.stakingKeeper.SetValidatorByConsAddr(f.sdkCtx, val2)
	f.stakingKeeper.SetNewValidatorByPowerIndex(f.sdkCtx, val1)
	f.stakingKeeper.SetNewValidatorByPowerIndex(f.sdkCtx, val2)

	_, err := f.stakingKeeper.Delegate(f.sdkCtx, addrs[0], f.stakingKeeper.TokensFromConsensusPower(f.sdkCtx, powers[0]), types.Unbonded, val1, true)
	assert.NilError(t, err)
	_, err = f.stakingKeeper.Delegate(f.sdkCtx, addrs[1], f.stakingKeeper.TokensFromConsensusPower(f.sdkCtx, powers[1]), types.Unbonded, val2, true)
	assert.NilError(t, err)
	_, err = f.stakingKeeper.Delegate(f.sdkCtx, addrs[0], f.stakingKeeper.TokensFromConsensusPower(f.sdkCtx, powers[2]), types.Unbonded, val2, true)
	assert.NilError(t, err)
	applyValidatorSetUpdates(t, f.sdkCtx, f.stakingKeeper, -1)

	return addrs, valAddrs, vals
}

func initFixture(t testing.TB) *fixture {
	keys := storetypes.NewKVStoreKeys(
		authtypes.StoreKey, banktypes.StoreKey, types.StoreKey, distrtypes.StoreKey, slashingtypes.StoreKey,
	)
	cdc := moduletestutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, staking.AppModuleBasic{}).Codec

	logger := log.NewTestLogger(t)
	cms := integration.CreateMultiStore(keys, logger)

	newCtx := sdk.NewContext(cms, cmtprototypes.Header{}, true, logger)

	authority := authtypes.NewModuleAddress("gov")

	maccPerms := map[string][]string{
		minttypes.ModuleName:    {authtypes.Minter},
		types.ModuleName:        {authtypes.Minter},
		types.BondedPoolName:    {authtypes.Burner, authtypes.Staking},
		types.NotBondedPoolName: {authtypes.Burner, authtypes.Staking},
		distrtypes.ModuleName:   {authtypes.Minter},
	}

	accountKeeper := authkeeper.NewAccountKeeper(
		cdc,
		runtime.NewKVStoreService(keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		maccPerms,
		sdk.Bech32MainPrefix,
		authority.String(),
	)

	blockedAddresses := map[string]bool{
		accountKeeper.GetAuthority(): false,
	}
	bankKeeper := bankkeeper.NewBaseKeeper(
		cdc,
		runtime.NewKVStoreService(keys[banktypes.StoreKey]),
		accountKeeper,
		blockedAddresses,
		authority.String(),
		log.NewNopLogger(),
	)

	stakingKeeper := stakingkeeper.NewKeeper(cdc, keys[types.StoreKey], accountKeeper, bankKeeper, authority.String())

	distrKeeper := distrkeeper.NewKeeper(
		cdc, runtime.NewKVStoreService(keys[distrtypes.StoreKey]), accountKeeper, bankKeeper, stakingKeeper, distrtypes.ModuleName, authority.String(),
	)
	distrKeeper.SetFeePool(newCtx, distrtypes.FeePool{
		CommunityPool: sdk.NewDecCoins(sdk.DecCoin{Denom: sdk.DefaultBondDenom, Amount: math.LegacyNewDec(0)}),
	})

	slashingKeeper := slashingkeeper.NewKeeper(cdc, codec.NewLegacyAmino(), keys[slashingtypes.StoreKey], stakingKeeper, authority.String())
	stakingKeeper.SetHooks(
		types.NewMultiStakingHooks(distrKeeper.Hooks(), slashingKeeper.Hooks()),
	)

	authModule := auth.NewAppModule(cdc, accountKeeper, authsims.RandomGenesisAccounts, nil)
	bankModule := bank.NewAppModule(cdc, bankKeeper, accountKeeper, nil)
	stakingModule := staking.NewAppModule(cdc, stakingKeeper, accountKeeper, bankKeeper, nil)
	distrModule := distribution.NewAppModule(cdc, distrKeeper, accountKeeper, bankKeeper, stakingKeeper, nil)
	slashingModule := slashing.NewAppModule(cdc, slashingKeeper, accountKeeper, bankKeeper, stakingKeeper, nil, cdc.InterfaceRegistry())

	integrationApp := integration.NewIntegrationApp(newCtx, logger, keys, cdc, authModule, bankModule, stakingModule, distrModule, slashingModule)

	sdkCtx := sdk.UnwrapSDKContext(integrationApp.Context())

	// Register MsgServer and QueryServer
	types.RegisterMsgServer(integrationApp.MsgServiceRouter(), stakingkeeper.NewMsgServerImpl(stakingKeeper))
	types.RegisterQueryServer(integrationApp.QueryHelper(), stakingkeeper.NewQuerier(stakingKeeper))

	// set default staking params
	stakingKeeper.SetParams(sdkCtx, types.DefaultParams())

	f := fixture{
		app:           integrationApp,
		sdkCtx:        sdkCtx,
		cdc:           cdc,
		keys:          keys,
		accountKeeper: accountKeeper,
		bankKeeper:    bankKeeper,
		stakingKeeper: stakingKeeper,
	}

	return &f
}