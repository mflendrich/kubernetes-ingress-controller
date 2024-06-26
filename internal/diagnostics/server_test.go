package diagnostics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/kong/go-database-reconciler/pkg/file"
	"github.com/stretchr/testify/require"

	"github.com/kong/kubernetes-ingress-controller/v3/internal/util"
	testhelpers "github.com/kong/kubernetes-ingress-controller/v3/test/helpers"
)

// TestDiagnosticsServer_ConfigDumps tests that the diagnostics server can receive and serve config dumps.
// It's primarily to test that write and read operations run simultaneously do not fall into a race condition.
func TestDiagnosticsServer_ConfigDumps(t *testing.T) {
	s := NewServer(logr.Discard(), ServerConfig{
		ConfigDumpsEnabled: true,
	})
	configsCh := s.configDumps.Configs

	port := testhelpers.GetFreePort(t)
	t.Logf("Obtained a free port: %d", port)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := s.Listen(ctx, port)
		require.NoError(t, err)
	}()
	t.Log("Started diagnostics server")

	// Use a WaitGroup to ensure that both the write and read operations are run simultaneously.
	readWriteWg := sync.WaitGroup{}
	readWriteWg.Add(2)

	// Write 1000 config dumps to the Server.
	const configDumpsToWrite = 1000
	go func() {
		readWriteWg.Done()
		readWriteWg.Wait()

		defer cancel()
		failed := false
		for i := 0; i < configDumpsToWrite; i++ {
			failed = !failed // Toggle failed flag.
			configsCh <- util.ConfigDump{
				Config:          file.Content{},
				Failed:          failed,
				RawResponseBody: []byte("fake error body"),
			}
		}
	}()
	t.Log("Started writing config dumps")

	// Continuously read config dumps from the Server until context is cancelled.
	go func() {
		readWriteWg.Done()
		readWriteWg.Wait()

		httpClient := &http.Client{}
		for {
			select {
			case <-ctx.Done():
				return
			default:
				resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/debug/config/successful", port))
				if err == nil {
					_ = resp.Body.Close()
				}
				resp, err = httpClient.Get(fmt.Sprintf("http://localhost:%d/debug/config/failed", port))
				if err == nil {
					_ = resp.Body.Close()
				}
				resp, err = httpClient.Get(fmt.Sprintf("http://localhost:%d/debug/config/raw-error", port))
				if err == nil {
					_ = resp.Body.Close()
				}
			}
		}
	}()
	t.Log("Started reading config dumps")

	<-ctx.Done()
}
