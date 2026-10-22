package audit_test

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/76Parker/metrico/internal/adapters/audit"
	"github.com/76Parker/metrico/internal/applications/auditobserver"
)

func TestFile_Notify_AppendsJSONLines(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("existing line\n"), 0o600))
	receiver, err := audit.NewFile(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, receiver.Close()) })
	event := auditobserver.Event{TS: 12345678, Metrics: []string{"Alloc", "Frees"}, IPAddress: "192.168.0.42"}
	require.NoError(t, receiver.Notify(context.Background(), event))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")
	require.Equal(t, "existing line", lines[0])
	require.JSONEq(t, `{"ts":12345678,"metrics":["Alloc","Frees"],"ip_address":"192.168.0.42"}`, lines[1])
	require.Equal(t, "", lines[2])
}

func TestFile_Notify_SerializesConcurrentWrites(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	receiver, err := audit.NewFile(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, receiver.Close()) })
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	const eventCount = 64
	group, ctx := errgroup.WithContext(context.Background())
	for i := range eventCount {
		group.Go(func() error {
			return receiver.Notify(ctx, auditobserver.Event{
				TS:        int64(i),
				Metrics:   []string{strconv.Itoa(i)},
				IPAddress: "::1",
			})
		})
	}
	require.NoError(t, group.Wait())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(data), "\n"))
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	received := make(map[int64]bool, eventCount)
	for scanner.Scan() {
		var event auditobserver.Event
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &event))
		require.False(t, received[event.TS])
		require.Equal(t, []string{strconv.FormatInt(event.TS, 10)}, event.Metrics)
		require.Equal(t, "::1", event.IPAddress)
		received[event.TS] = true
	}
	require.NoError(t, scanner.Err())
	require.Len(t, received, eventCount)
}

func TestFile_Notify_ReturnsCancellationAndClosedFileErrors(t *testing.T) {
	t.Parallel()
	receiver, err := audit.NewFile(filepath.Join(t.TempDir(), "audit.jsonl"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, receiver.Close()) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, receiver.Notify(ctx, auditobserver.Event{}), context.Canceled)
	require.NoError(t, receiver.Close())
	require.ErrorIs(t, receiver.Notify(context.Background(), auditobserver.Event{}), os.ErrClosed)
}

func TestNewFile_ReturnsOpenError(t *testing.T) {
	t.Parallel()
	_, err := audit.NewFile(filepath.Join(t.TempDir(), "missing", "audit.jsonl"))
	require.ErrorIs(t, err, os.ErrNotExist)
}
