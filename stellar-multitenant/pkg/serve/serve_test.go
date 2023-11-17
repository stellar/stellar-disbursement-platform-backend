package serve

import (
	"testing"
	"time"

	"github.com/stellar/go/network"
	supporthttp "github.com/stellar/go/support/http"
	"github.com/stellar/stellar-disbursement-platform-backend/db"
	"github.com/stellar/stellar-disbursement-platform-backend/db/dbtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockHTTPServer struct {
	mock.Mock
}

func (m *mockHTTPServer) Run(conf supporthttp.Config) {
	m.Called(conf)
}

var _ HTTPServerInterface = new(mockHTTPServer)

func Test_Serve(t *testing.T) {
	dbt := dbtest.OpenWithoutMigrations(t)
	defer dbt.Close()

	dbConnectionPool, err := db.OpenDBConnectionPool(dbt.DSN)
	require.NoError(t, err)
	defer dbConnectionPool.Close()

	opts := ServeOptions{
		DatabaseDSN:       dbt.DSN,
		Environment:       "test",
		GitCommit:         "1234567890abcdef",
		NetworkPassphrase: network.TestNetworkPassphrase,
		Port:              8003,
		Version:           "x.y.z",
	}

	// Mock supportHTTPRun
	mHTTPServer := mockHTTPServer{}
	mHTTPServer.On("Run", mock.AnythingOfType("http.Config")).Run(func(args mock.Arguments) {
		conf, ok := args.Get(0).(supporthttp.Config)
		require.True(t, ok, "should be of type supporthttp.Config")
		assert.Equal(t, ":8003", conf.ListenAddr)
		assert.Equal(t, time.Minute*3, conf.TCPKeepAlive)
		assert.Equal(t, time.Second*50, conf.ShutdownGracePeriod)
		assert.Equal(t, time.Second*5, conf.ReadTimeout)
		assert.Equal(t, time.Second*35, conf.WriteTimeout)
		assert.Equal(t, time.Minute*2, conf.IdleTimeout)
		assert.Nil(t, conf.TLS)
		assert.ObjectsAreEqualValues(handleHTTP(&opts), conf.Handler)
		conf.OnStopping()
	}).Once()

	// test and assert
	err = StartServe(opts, &mHTTPServer)
	require.NoError(t, err)
	mHTTPServer.AssertExpectations(t)
}
