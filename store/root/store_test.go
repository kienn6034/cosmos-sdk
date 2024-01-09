package root

import (
	"fmt"
	"io"
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/suite"

	"cosmossdk.io/log"
	"cosmossdk.io/store/v2"
	"cosmossdk.io/store/v2/commitment"
	"cosmossdk.io/store/v2/commitment/iavl"
	"cosmossdk.io/store/v2/pruning"
	"cosmossdk.io/store/v2/storage"
	"cosmossdk.io/store/v2/storage/sqlite"
)

const (
	testStoreKey = "test"
)

type RootStoreTestSuite struct {
	suite.Suite

	rootStore store.RootStore
}

func TestStorageTestSuite(t *testing.T) {
	suite.Run(t, &RootStoreTestSuite{})
}

func (s *RootStoreTestSuite) SetupTest() {
	noopLog := log.NewNopLogger()

	sqliteDB, err := sqlite.New(s.T().TempDir())
	s.Require().NoError(err)
	ss := storage.NewStorageStore(sqliteDB)

	tree := iavl.NewIavlTree(dbm.NewMemDB(), noopLog, iavl.DefaultConfig())
	sc, err := commitment.NewCommitStore(map[string]commitment.Tree{testStoreKey: tree}, dbm.NewMemDB(), noopLog)
	s.Require().NoError(err)

	rs, err := New(noopLog, ss, sc, []string{testStoreKey}, pruning.DefaultOptions(), pruning.DefaultOptions(), nil)
	s.Require().NoError(err)

	rs.SetTracer(io.Discard)
	rs.SetTracingContext(store.TraceContext{
		"test": s.T().Name(),
	})

	s.rootStore = rs
}

func (s *RootStoreTestSuite) TearDownTest() {
	err := s.rootStore.Close()
	s.Require().NoError(err)
}

func (s *RootStoreTestSuite) TestGetSCStore() {
	s.Require().Equal(s.rootStore.GetSCStore(), s.rootStore.(*Store).stateCommitment)
}

func (s *RootStoreTestSuite) TestGetKVStore() {
	kvs := s.rootStore.GetKVStore(testStoreKey)
	s.Require().NotNil(kvs)
}

func (s *RootStoreTestSuite) TestQuery() {
	_, err := s.rootStore.Query("", 1, []byte("foo"), true)
	s.Require().Error(err)

	// write and commit a changeset
	bs := s.rootStore.GetKVStore(testStoreKey)
	bs.Set([]byte("foo"), []byte("bar"))

	workingHash, err := s.rootStore.WorkingHash()
	s.Require().NoError(err)
	s.Require().NotNil(workingHash)

	commitHash, err := s.rootStore.Commit()
	s.Require().NoError(err)
	s.Require().NotNil(commitHash)
	s.Require().Equal(workingHash, commitHash)

	// ensure the proof is non-nil for the corresponding version
	result, err := s.rootStore.Query(testStoreKey, 1, []byte("foo"), true)
	s.Require().NoError(err)
	s.Require().NotNil(result.ProofOps)
	s.Require().Equal([]byte("foo"), result.ProofOps[0].Key)
}

// func (s *RootStoreTestSuite) TestQueryProof() {
// 	// store1
// 	bs1 := s.rootStore.GetBranchedKVStore("store1")
// 	bs1.Set([]byte("key1"), []byte("value1"))
// 	bs1.Set([]byte("key2"), []byte("value2"))

// 	// store2
// 	bs2 := s.rootStore.GetBranchedKVStore("store2")
// 	bs2.Set([]byte("key3"), []byte("value3"))

// 	// store3
// 	bs3 := s.rootStore.GetBranchedKVStore("store3")
// 	bs3.Set([]byte("key4"), []byte("value4"))

// 	// commit
// 	_, err := s.rootStore.WorkingHash()
// 	s.Require().NoError(err)
// 	_, err = s.rootStore.Commit()
// 	s.Require().NoError(err)

// 	// query proof for store1
// 	result, err := s.rootStore.Query("store1", 1, []byte("key1"), true)
// 	s.Require().NoError(err)
// 	s.Require().NotNil(result.ProofOps)
// 	cInfo, err := s.rootStore.GetSCStore().GetCommitInfo(1)
// 	s.Require().NoError(err)
// 	storeHash := cInfo.GetStoreCommitID("store1").Hash
// 	treeRoots, err := result.ProofOps[0].Run([][]byte{[]byte("value1")})
// 	s.Require().NoError(err)
// 	s.Require().Equal(treeRoots[0], storeHash)
// 	expRoots, err := result.ProofOps[1].Run([][]byte{storeHash})
// 	s.Require().NoError(err)
// 	s.Require().Equal(expRoots[0], cInfo.Hash())
// }

func (s *RootStoreTestSuite) TestLoadVersion() {
	// write and commit a few changesets
	for v := 1; v <= 5; v++ {
		bs := s.rootStore.GetKVStore(testStoreKey)
		val := fmt.Sprintf("val%03d", v) // val001, val002, ..., val005
		bs.Set([]byte("key"), []byte(val))

		workingHash, err := s.rootStore.WorkingHash()
		s.Require().NoError(err)
		s.Require().NotNil(workingHash)

		commitHash, err := s.rootStore.Commit()
		s.Require().NoError(err)
		s.Require().NotNil(commitHash)
		s.Require().Equal(workingHash, commitHash)
	}

	// ensure the latest version is correct
	latest, err := s.rootStore.GetLatestVersion()
	s.Require().NoError(err)
	s.Require().Equal(uint64(5), latest)

	// attempt to load a non-existent version
	err = s.rootStore.LoadVersion(6)
	s.Require().Error(err)

	// attempt to load a previously committed version
	err = s.rootStore.LoadVersion(3)
	s.Require().NoError(err)

	// ensure the latest version is correct
	latest, err = s.rootStore.GetLatestVersion()
	s.Require().NoError(err)
	s.Require().Equal(uint64(3), latest)

	// query state and ensure values returned are based on the loaded version
	kvStore := s.rootStore.GetKVStore(testStoreKey)
	val := kvStore.Get([]byte("key"))
	s.Require().Equal([]byte("val003"), val)

	// attempt to write and commit a few changesets
	for v := 4; v <= 5; v++ {
		bs := s.rootStore.GetKVStore(testStoreKey)
		val := fmt.Sprintf("overwritten_val%03d", v) // overwritten_val004, overwritten_val005
		bs.Set([]byte("key"), []byte(val))

		workingHash, err := s.rootStore.WorkingHash()
		s.Require().NoError(err)
		s.Require().NotNil(workingHash)

		commitHash, err := s.rootStore.Commit()
		s.Require().NoError(err)
		s.Require().NotNil(commitHash)
		s.Require().Equal(workingHash, commitHash)
	}

	// ensure the latest version is correct
	latest, err = s.rootStore.GetLatestVersion()
	s.Require().NoError(err)
	s.Require().Equal(uint64(5), latest)

	// query state and ensure values returned are based on the loaded version
	kvStore = s.rootStore.GetKVStore(testStoreKey)
	val = kvStore.Get([]byte("key"))
	s.Require().Equal([]byte("overwritten_val005"), val)
}

func (s *RootStoreTestSuite) TestCommit() {
	lv, err := s.rootStore.GetLatestVersion()
	s.Require().NoError(err)
	s.Require().Zero(lv)

	// perform changes
	bs2 := s.rootStore.GetKVStore(testStoreKey)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key%03d", i) // key000, key001, ..., key099
		val := fmt.Sprintf("val%03d", i) // val000, val001, ..., val099

		bs2.Set([]byte(key), []byte(val))
	}

	// committing w/o calling WorkingHash should error
	_, err = s.rootStore.Commit()
	s.Require().Error(err)

	// execute WorkingHash and Commit
	wHash, err := s.rootStore.WorkingHash()
	s.Require().NoError(err)

	cHash, err := s.rootStore.Commit()
	s.Require().NoError(err)
	s.Require().Equal(wHash, cHash)

	// ensure latest version is updated
	lv, err = s.rootStore.GetLatestVersion()
	s.Require().NoError(err)
	s.Require().Equal(uint64(1), lv)

	// ensure the root KVStore is cleared
	s.Require().Empty(s.rootStore.(*Store).kvStores[testStoreKey].GetChangeset().Size())

	// perform reads on the updated root store
	bs := s.rootStore.GetKVStore(testStoreKey)
	for i := 0; i < 100; i++ {
		key := fmt.Sprintf("key%03d", i) // key000, key001, ..., key099
		val := fmt.Sprintf("val%03d", i) // val000, val001, ..., val099

		s.Require().Equal([]byte(val), bs.Get([]byte(key)))
	}
}
