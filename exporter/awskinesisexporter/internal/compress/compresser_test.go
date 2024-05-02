// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package compress_test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/open-telemetry/opentelemetry-collector-contrib/exporter/awskinesisexporter/internal/compress"
)

func TestCompressorFormats(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		format string
	}{
		{format: "none"},
		{format: "noop"},
		{format: "gzip"},
		{format: "zlib"},
		{format: "flate"},
	}

	source := rand.NewSource(time.Now().UnixMilli())
	genRand := rand.New(source)

	data2 := make([]byte, 1065)
	for i := 0; i < 1065; i++ {
		data2[i] = byte(genRand.Int31())
	}

	const data = "You know nothing Jon Snow"
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("format_%s", tc.format), func(t *testing.T) {
			c, err := compress.NewCompressor(tc.format)
			require.NoError(t, err, "Must have a valid compression format")
			require.NotNil(t, c, "Must have a valid compressor")

			out, err := c([]byte(data2))
			assert.NoError(t, err, "Must not error when processing data")
			assert.NotNil(t, out, "Must have a valid record")

			if tc.format == "gzip" {
				dc, err2 := decompress(out)

				assert.NoError(t, err2)
				assert.Equal(t, data2, dc)
			}
		})
	}
	_, err := compress.NewCompressor("invalid-format")
	assert.Error(t, err, "Must error when an invalid compression format is given")
}

func decompress(input []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(input))
	if err != nil {
		return nil, err
	}

	defer r.Close()

	decompressedData, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return decompressedData, nil
}

func BenchmarkNoopCompressor_1000Bytes(b *testing.B) {
	benchmarkCompressor(b, "none", 1000)
}

func BenchmarkNoopCompressor_1Mb(b *testing.B) {
	benchmarkCompressor(b, "noop", 131072)
}

func BenchmarkZlibCompressor_1000Bytes(b *testing.B) {
	benchmarkCompressor(b, "zlib", 1000)
}

func BenchmarkZlibCompressor_1Mb(b *testing.B) {
	benchmarkCompressor(b, "zlib", 131072)
}

func BenchmarkFlateCompressor_1000Bytes(b *testing.B) {
	benchmarkCompressor(b, "flate", 1000)
}

func BenchmarkFlateCompressor_1Mb(b *testing.B) {
	benchmarkCompressor(b, "flate", 131072)
}

func BenchmarkGzipCompressor_1000Bytes(b *testing.B) {
	benchmarkCompressor(b, "gzip", 1000)
}

func BenchmarkGzipCompressor_1Mb(b *testing.B) {
	benchmarkCompressor(b, "gzip", 131072)
}

func benchmarkCompressor(b *testing.B, format string, length int) {
	b.Helper()

	source := rand.NewSource(time.Now().UnixMilli())
	genRand := rand.New(source)

	compressor, err := compress.NewCompressor(format)
	require.NoError(b, err, "Must not error when given a valid format")
	require.NotNil(b, compressor, "Must have a valid compressor")

	data := make([]byte, length)
	for i := 0; i < length; i++ {
		data[i] = byte(genRand.Int31())
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		out, err := compressor(data)
		assert.NoError(b, err, "Must not error when processing data")
		assert.NotNil(b, out, "Must have a valid byte array after")
	}
}

// an issue encountered in the past was a crash due race condition in the compressor, so the
// current implementation creates a new context on each compression request
// this is a test to check no exceptions are raised for executing concurrent compressions
func TestCompressorConcurrent(t *testing.T) {

	timeout := time.After(15 * time.Second)
	done := make(chan bool)
	go func() {
		// do your testing
		concurrentCompressFunc(t)
		done <- true
	}()

	select {
	case <-timeout:
		t.Fatal("Test didn't finish in time")
	case <-done:
	}

}

func concurrentCompressFunc(t *testing.T) {
	// this value should be way higher to make this test more valuable, but the make of this project uses
	// max 4 workers, so we had to set this value here
	numWorkers := 4

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	errCh := make(chan error, numWorkers)
	var errMutex sync.Mutex

	// any single format would do it here, since each exporter can be set to use only one at a time
	// and the concurrent issue that was present in the past was independent of the format
	compressFunc, err := compress.NewCompressor("gzip")

	if err != nil {
		errCh <- err
		return
	}

	// it is important for the data length to be on the higher side of a record
	// since it is where the chances of having race conditions are bigger
	dataLength := 131072

	for j := 0; j < numWorkers; j++ {
		go func() {
			defer wg.Done()

			source := rand.NewSource(time.Now().UnixMilli())
			genRand := rand.New(source)

			data := make([]byte, dataLength)
			for i := 0; i < dataLength; i++ {
				data[i] = byte(genRand.Int31())
			}

			result, localErr := compressFunc(data)
			if localErr != nil {
				errMutex.Lock()
				errCh <- localErr
				errMutex.Unlock()
				return
			}

			_ = result
		}()
	}

	wg.Wait()

	close(errCh)

	for err := range errCh {
		t.Errorf("Error encountered on concurrent compression: %v", err)
	}
}
